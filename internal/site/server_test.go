package site

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/titagaki/peercast-mi/internal/channel"
	"github.com/titagaki/peercast-mi/internal/config"
	"github.com/titagaki/peercast-pcp/pcp"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func testSite(t *testing.T) *Server {
	t.Helper()
	t.Setenv("PEERCAST_X_CLIENT_ID", "test-client")
	t.Setenv("PEERCAST_X_CLIENT_SECRET", "test-secret")
	m := channel.NewManager(pcp.GnuID{})
	m.SetCachePath(filepath.Join(t.TempDir(), "keys.json"))
	t.Cleanup(m.StopAll)
	s, e := New(config.Site{Origin: "https://live.example", RTMPURL: "rtmps://live.example/live", MaxViewers: 2, MaxViewersPerUser: 1}, m, 7144, nil)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func addSession(s *Server, cookie, id string) *session {
	ss := &session{User: user{id, "User " + id}, CSRF: "csrf-" + cookie, Expires: time.Now().Add(time.Hour), Done: make(chan struct{})}
	s.mu.Lock()
	s.sessions[cookie] = ss
	s.mu.Unlock()
	return ss
}
func call(s *Server, method, path, cookie, csrf, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Origin", s.cfg.Origin)
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	}
	if csrf != "" {
		r.Header.Set("X-CSRF-Token", csrf)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}
func TestAuthenticationAndCSRF(t *testing.T) {
	s := testSite(t)
	ss := addSession(s, "alice", "1")
	for _, path := range []string{"/site/api/channels", "/site/api/broadcast", "/site/stream/01234567890123456789012345678901"} {
		if w := call(s, "GET", path, "", "", ""); w.Code != 401 {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
	if w := call(s, "POST", "/site/api/key", "alice", "", ""); w.Code != 403 {
		t.Fatal(w.Code)
	}
	r := httptest.NewRequest("POST", "/site/api/key", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: "alice"})
	r.Header.Set("X-CSRF-Token", ss.CSRF)
	r.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w := call(s, "POST", "/api/1", "alice", ss.CSRF, `{"method":"listStreamKeys"}`); w.Code != 404 {
		t.Fatal(w.Code)
	}
	ss.Expires = time.Now().Add(-time.Second)
	if w := call(s, "GET", "/site/api/channels", "alice", "", ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	select {
	case <-ss.Done:
	default:
		t.Fatal("expired session not closed")
	}
}
func TestOwnershipAndRotation(t *testing.T) {
	s := testSite(t)
	a := addSession(s, "alice", "1")
	b := addSession(s, "bob", "2")
	for _, v := range []struct {
		cookie string
		ss     *session
	}{{"alice", a}, {"bob", b}} {
		if w := call(s, "POST", "/site/api/key", v.cookie, v.ss.CSRF, ""); w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	old := s.key(a)
	if old == "" || old == s.key(b) {
		t.Fatal("keys not isolated")
	}
	w := call(s, "POST", "/site/api/broadcast", "alice", a.CSRF, `{"name":"Alice","genre":"Music","description":"live"}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	ch, _ := s.mgr.GetByStreamKey(old)
	for _, path := range []string{"/site/api/channels", "/site/api/broadcast"} {
		w := call(s, "GET", path, "bob", "", "")
		if strings.Contains(w.Body.String(), old) {
			t.Fatal("other user's key leaked")
		}
	}
	// Supplying another user's key/account is rejected, not interpreted.
	if w := call(s, "POST", "/site/api/broadcast", "bob", b.CSRF, `{"name":"evil","streamKey":"`+old+`"}`); w.Code != 400 {
		t.Fatal(w.Code)
	}
	call(s, "DELETE", "/site/api/broadcast", "bob", b.CSRF, "")
	if _, ok := s.mgr.GetByID(ch.ID); !ok {
		t.Fatal("Bob stopped Alice")
	}
	if w := call(s, "POST", "/site/api/key", "alice", a.CSRF, ""); w.Code != 409 {
		t.Fatal("rotation allowed while live")
	}
	call(s, "DELETE", "/site/api/broadcast", "alice", a.CSRF, "")
	if w := call(s, "POST", "/site/api/key", "alice", a.CSRF, ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if s.mgr.IsIssuedKey(old) {
		t.Fatal("old key still valid")
	}
	// Ownership survives a new browser session because it is based on X ID.
	a2 := addSession(s, "alice2", "1")
	if s.key(a2) != s.key(a) {
		t.Fatal("identity not stable")
	}
}
func TestKeySaveFailure(t *testing.T) {
	s := testSite(t)
	a := addSession(s, "alice", "1")
	call(s, "POST", "/site/api/key", "alice", a.CSRF, "")
	old := s.key(a)
	s.mgr.SetCachePath(filepath.Join(t.TempDir(), "missing", "keys.json"))
	if w := call(s, "POST", "/site/api/key", "alice", a.CSRF, ""); w.Code != 500 {
		t.Fatal(w.Code)
	}
	if s.key(a) != old {
		t.Fatal("failed persistence replaced key")
	}
}
func TestOAuthPKCECallbackAndReplay(t *testing.T) {
	s := testSite(t)
	w := call(s, "GET", "/auth/x/start", "", "", "")
	if w.Code != 302 {
		t.Fatal(w.Code)
	}
	u, _ := url.Parse(w.Header().Get("Location"))
	q := u.Query()
	if q.Get("code_challenge_method") != "S256" || len(q.Get("code_challenge")) != 43 {
		t.Fatal(q)
	}
	var cookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == flowCookie {
			cookie = c
		}
	}
	if cookie == nil || !cookie.Secure || !cookie.HttpOnly {
		t.Fatal("unsafe flow cookie")
	}
	f := s.flows[cookie.Value]
	calls := 0
	s.client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		body := `{"data":{"id":"1234","name":"Changed display name"}}`
		if r.URL.String() == s.tokenURL {
			r.ParseForm()
			if r.Form.Get("code_verifier") != f.Verifier || r.Form.Get("redirect_uri") != s.cfg.Origin+"/auth/x/callback" {
				t.Error("PKCE/callback mismatch")
			}
			id, secret, _ := r.BasicAuth()
			if id != "test-client" || secret != "test-secret" {
				t.Error("client auth mismatch")
			}
			body = `{"access_token":"provider-secret","token_type":"bearer"}`
		} else if r.Header.Get("Authorization") != "Bearer provider-secret" {
			t.Error("missing user token")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	r := httptest.NewRequest("GET", "/auth/x/callback?state="+q.Get("state")+"&code=code", nil)
	r.AddCookie(cookie)
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 303 || calls != 2 {
		t.Fatal(w.Code, w.Body.String(), calls)
	}
	var sid string
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookie {
			sid = c.Value
			if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode {
				t.Fatal("unsafe session")
			}
		}
	}
	if s.sessions[sid] == nil || s.sessions[sid].User.ID != "1234" {
		t.Fatal("no session")
	}
	if strings.Contains(w.Body.String(), "provider-secret") {
		t.Fatal("token leaked")
	}
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 400 || calls != 2 {
		t.Fatal("callback replay accepted")
	}
}
func TestOAuthRejectsInvalidStateAndProviderFailure(t *testing.T) {
	for _, kind := range []string{"state", "expired", "denied", "provider"} {
		t.Run(kind, func(t *testing.T) {
			s := testSite(t)
			f := flow{"correct", "verifier", time.Now().Add(time.Minute)}
			if kind == "expired" {
				f.Expires = time.Now().Add(-time.Second)
			}
			s.flows["flow"] = f
			q := url.Values{"state": {"correct"}, "code": {"code"}}
			if kind == "state" {
				q.Set("state", "wrong")
			}
			if kind == "denied" {
				q.Set("error", "access_denied")
			}
			calls := 0
			s.client.Transport = transportFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, context.DeadlineExceeded })
			r := httptest.NewRequest("GET", "/auth/x/callback?"+q.Encode(), nil)
			r.AddCookie(&http.Cookie{Name: flowCookie, Value: "flow"})
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			want := 400
			if kind == "provider" {
				want = 502
			}
			if w.Code != want || len(s.sessions) != 0 {
				t.Fatal(w.Code)
			}
			if kind != "provider" && calls != 0 {
				t.Fatal("called provider before state validation")
			}
		})
	}
}
func TestStreamLimitsSanitizationAndLogout(t *testing.T) {
	s := testSite(t)
	a := addSession(s, "alice", "1")
	addSession(s, "alice2", "1")
	b := addSession(s, "bob", "2")
	addSession(s, "carol", "3")
	s.mgr.IssueStreamKey("source", "secret")
	ch, _ := s.mgr.Broadcast("secret", channel.ChannelInfo{Name: "Live"}, channel.TrackInfo{})
	path := "/site/stream/" + view(ch).ID
	entered := make(chan struct{}, 2)
	s.proxy.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "Bearer "+s.ViewerToken() || r.URL.RawQuery != "" || r.URL.Host != "127.0.0.1:7144" {
			t.Error("unsafe proxy request")
		}
		entered <- struct{}{}
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	start := func(cookie string) <-chan struct{} {
		done := make(chan struct{})
		go func() { call(s, "GET", path, cookie, "", ""); close(done) }()
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("stream not started")
		}
		return done
	}
	da := start("alice")
	if w := call(s, "GET", path, "alice2", "", ""); w.Code != 429 {
		t.Fatal("per-user limit not shared across sessions", w.Code)
	}
	db := start("bob")
	if w := call(s, "GET", path, "carol", "", ""); w.Code != 429 {
		t.Fatal("global limit not applied")
	}
	if w := call(s, "GET", path+"?tip=localhost:1234", "alice", "", ""); w.Code != 400 {
		t.Fatal("arbitrary relay request allowed")
	}
	for _, v := range []struct {
		cookie string
		ss     *session
		done   <-chan struct{}
	}{{"alice", a, da}, {"bob", b, db}} {
		call(s, "POST", "/site/api/logout", v.cookie, v.ss.CSRF, "")
		select {
		case <-v.done:
		case <-time.After(time.Second):
			t.Fatal("logout left media running")
		}
	}
	if s.total != 0 || len(s.viewers) != 0 {
		t.Fatal("viewer slot leak")
	}
}
func TestSessionResponseHasNoKey(t *testing.T) {
	s := testSite(t)
	addSession(s, "alice", "1")
	w := call(s, "GET", "/site/api/me", "alice", "", "")
	var v map[string]any
	if e := json.Unmarshal(w.Body.Bytes(), &v); e != nil {
		t.Fatal(e)
	}
	if v["csrf"] == nil || v["user"] == nil {
		t.Fatal(v)
	}
}

func TestConfigFailsClosed(t *testing.T) {
	s := testSite(t)
	for _, origin := range []string{"", "http://public.example", "https://user:pass@live.example", "https://live.example/path", "https://live.example?x=1"} {
		cfg := s.cfg
		cfg.Origin = origin
		if _, err := New(cfg, s.mgr, 7144, nil); err == nil {
			t.Errorf("accepted origin %q", origin)
		}
	}
	cfg := s.cfg
	cfg.MaxViewers = -1
	if _, err := New(cfg, s.mgr, 7144, nil); err == nil {
		t.Error("negative limit accepted")
	}
	t.Setenv("PEERCAST_X_CLIENT_SECRET", "")
	if _, err := New(s.cfg, s.mgr, 7144, nil); err == nil {
		t.Error("missing secret accepted")
	}
}
