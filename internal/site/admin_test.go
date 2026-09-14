package site

import (
	"github.com/titagaki/peercast-mi/internal/config"
	"github.com/titagaki/peercast-mi/internal/jsonrpc"
	"github.com/titagaki/peercast-pcp/pcp"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAdminRPCRequiresAllowlistSessionAndCSRF(t *testing.T) {
	s := testSite(t)
	s.cfg.BasePath = "/mi"
	s.adminIDs = map[string]bool{"123": true}
	owner := addSession(s, "owner", "123")
	viewer := addSession(s, "viewer", "456")
	calls := 0
	s.adminProxy.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != "http://127.0.0.1:7144/api/1" {
			t.Error(r.URL)
		}
		for _, name := range []string{"Authorization", "Cookie", "Origin", "X-CSRF-Token", "X-Forwarded-For"} {
			if r.Header.Get(name) != "" {
				t.Errorf("forwarded %s", name)
			}
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"stopChannel"`) {
			t.Error(string(body))
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","id":1,"result":null}`))}, nil
	})
	for _, tc := range []struct {
		name, cookie, csrf, origin string
		status                     int
	}{
		{"anonymous", "", "", s.cfg.Origin, 401},
		{"viewer", "viewer", viewer.CSRF, s.cfg.Origin, 403},
		{"missing csrf", "owner", "", s.cfg.Origin, 403},
		{"foreign origin", "owner", owner.CSRF, "https://evil.test", 403},
		{"missing origin", "owner", owner.CSRF, "", 403},
		{"owner", "owner", owner.CSRF, s.cfg.Origin, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/mi/admin/api/1", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"stopChannel","params":["123"]}`))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("X-CSRF-Token", tc.csrf)
			r.Header.Set("Authorization", "Basic ignored")
			r.Header.Set("X-Forwarded-For", "203.0.113.1")
			if tc.cookie != "" {
				r.AddCookie(&http.Cookie{Name: sessionCookie, Value: tc.cookie})
			}
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("%d: %s", w.Code, w.Body.String())
			}
		})
	}
	if calls != 1 {
		t.Fatalf("backend called %d times", calls)
	}
	if w := call(s, "GET", "/mi/site/api/me", "owner", "", ""); !strings.Contains(w.Body.String(), `"admin":true`) {
		t.Fatal(w.Body.String())
	}
	if w := call(s, "GET", "/mi/site/api/me", "viewer", "", ""); !strings.Contains(w.Body.String(), `"admin":false`) {
		t.Fatal(w.Body.String())
	}
	s.cfg.DevLogin = true
	if s.isAdmin(owner) {
		t.Fatal("development login gained admin access")
	}
	s.cfg.DevLogin = false
	s.adminIDs = nil
	if s.isAdmin(owner) {
		t.Fatal("empty allowlist gained admin access")
	}
}

func TestAdminAllowlistConfigAndLoginReturn(t *testing.T) {
	s := testSite(t)
	s.cfg.AdminXIDs = []string{"123", "456"}
	configured, err := New(s.cfg, s.mgr, 7144, nil)
	if err != nil || !configured.adminIDs["123"] || !configured.adminIDs["456"] {
		t.Fatal(configured, err)
	}
	s.cfg.AdminXIDs = []string{"@name"}
	if _, err := New(s.cfg, s.mgr, 7144, nil); err == nil {
		t.Fatal("accepted nonnumeric ID")
	}
	s.cfg.BasePath = "/mi"
	w := call(s, "GET", "/mi/auth/x/start?next=/mi/admin", "", "", "")
	u, _ := url.Parse(w.Header().Get("Location"))
	if u.Query().Get("redirect_uri") != s.cfg.Origin+"/mi/auth/x/callback" {
		t.Fatal(u)
	}
	for _, f := range s.flows {
		if f.Next != "/mi/admin" {
			t.Fatal(f.Next)
		}
	}
}

func TestAdminGatewayDispatchAndLogout(t *testing.T) {
	s := testSite(t)
	backend := httptest.NewServer(jsonrpc.New(pcp.GnuID{}, s.mgr, &config.Config{}, nil).Handler())
	defer backend.Close()
	u, _ := url.Parse(backend.URL)
	port, _ := strconv.Atoi(u.Port())
	cfg := s.cfg
	cfg.BasePath = "/mi"
	cfg.AdminXIDs = []string{"123"}
	s, err := New(cfg, s.mgr, port, nil)
	if err != nil {
		t.Fatal(err)
	}
	ss := addSession(s, "owner", "123")
	if err := s.mgr.IssueStreamKey("sample", "test-key"); err != nil {
		t.Fatal(err)
	}
	request := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/mi/admin/api/1", strings.NewReader(body))
		r.Header.Set("Origin", cfg.Origin)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", ss.CSRF)
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: "owner"})
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w
	}
	w := request(`{"jsonrpc":"2.0","id":1,"method":"listStreamKeys","params":[]}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "test-key") {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(`{"jsonrpc":"2.0","id":2,"method":"revokeStreamKey","params":["sample"]}`)
	if w.Code != 200 || s.mgr.IsIssuedKey("test-key") {
		t.Fatal(w.Code, w.Body.String())
	}
	call(s, "POST", "/mi/site/api/logout", "owner", ss.CSRF, "")
	if w = request(`{"jsonrpc":"2.0","id":3,"method":"listStreamKeys"}`); w.Code != 401 {
		t.Fatal(w.Code)
	}
	ss = addSession(s, "owner", "123")
	ss.Expires = time.Now().Add(-time.Second)
	if w = request(`{"jsonrpc":"2.0","id":4,"method":"listStreamKeys"}`); w.Code != 401 {
		t.Fatal(w.Code)
	}
}
