package site

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/titagaki/peercast-mi/internal/channel"
	"github.com/titagaki/peercast-mi/internal/config"
	"github.com/titagaki/peercast-pcp/pcp"
)

func newDevelopmentSite(t *testing.T) *Server {
	t.Helper()
	t.Setenv("PEERCAST_X_CLIENT_ID", "")
	t.Setenv("PEERCAST_X_CLIENT_SECRET", "")
	mgr := channel.NewManager(pcp.GnuID{})
	t.Cleanup(mgr.StopAll)
	s, err := New(config.Site{DevLogin: true, Origin: "http://localhost:5173", Listen: "127.0.0.1:8080", RTMPURL: "rtmp://localhost:1945/live"}, mgr, 7144, nil)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func devRequest(s *Server, method, path string) *http.Request {
	r := httptest.NewRequest(method, s.cfg.Origin+path, nil)
	r.RemoteAddr = "127.0.0.1:50000"
	if method == "POST" {
		r.Header.Set("Origin", s.cfg.Origin)
	}
	return r
}

func TestDevelopmentLoginCreatesOrdinaryIsolatedSession(t *testing.T) {
	s := newDevelopmentSite(t)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, devRequest(s, "GET", "/site/api/me"))
	if !strings.Contains(w.Body.String(), `"devLogin":true`) {
		t.Fatal(w.Body.String())
	}
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, devRequest(s, "GET", "/site/api/channels"))
	if w.Code != 401 {
		t.Fatal("anonymous access allowed")
	}
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, devRequest(s, "POST", "/site/api/dev-login"))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != sessionCookie || !cookies[0].HttpOnly {
		t.Fatal("missing session cookie")
	}
	ss := s.sessions[cookies[0].Value]
	if ss == nil || account(ss) != "site:dev:local" || account(ss) == account(&session{User: user{ID: "123"}}) {
		t.Fatal("identity not isolated")
	}
	r := devRequest(s, "POST", "/site/api/key")
	r.AddCookie(cookies[0])
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("CSRF bypassed")
	}
	r.Header.Set("X-CSRF-Token", ss.CSRF)
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var result map[string]string
	json.Unmarshal(w.Body.Bytes(), &result)
	if result["streamKey"] == "" || !s.mgr.IsIssuedKey(result["streamKey"]) {
		t.Fatal("key not usable on real manager")
	}
	r = devRequest(s, "POST", "/site/api/logout")
	r.AddCookie(cookies[0])
	r.Header.Set("X-CSRF-Token", ss.CSRF)
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	select {
	case <-ss.Done:
	default:
		t.Fatal("logout didn't invalidate session")
	}
	if _, ok := s.sessions[cookies[0].Value]; ok {
		t.Fatal("session survived logout")
	}
}

func TestDevelopmentLoginRejectsUnsafeRequests(t *testing.T) {
	s := newDevelopmentSite(t)
	for _, kind := range []string{"no-origin", "foreign-origin", "foreign-host", "foreign-peer", "forwarded-peer"} {
		t.Run(kind, func(t *testing.T) {
			r := devRequest(s, "POST", "/site/api/dev-login")
			switch kind {
			case "no-origin":
				r.Header.Del("Origin")
			case "foreign-origin":
				r.Header.Set("Origin", "https://evil.example")
			case "foreign-host":
				r.Host = "evil.example"
			case "foreign-peer", "forwarded-peer":
				r.RemoteAddr = "192.0.2.1:50000"
				r.Header.Set("X-Forwarded-For", "127.0.0.1")
			}
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			if w.Code != 403 {
				t.Fatal(kind, w.Code)
			}
		})
	}
	if len(s.sessions) != 0 {
		t.Fatal("unsafe request created session")
	}
	for _, path := range []string{"/auth/x/start", "/auth/x/callback"} {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, devRequest(s, "GET", path))
		if w.Code != 404 {
			t.Fatal("X enabled in development mode", w.Code)
		}
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, devRequest(s, "GET", "/site/api/dev-login"))
	if w.Code != 405 {
		t.Fatal("GET login allowed")
	}
	regular := testSite(t)
	w = call(regular, "POST", "/site/api/dev-login", "", "", "")
	if w.Code != 404 {
		t.Fatal("development endpoint present in normal mode")
	}
}

func TestDevelopmentLoginRejectsPublicConfiguration(t *testing.T) {
	s := newDevelopmentSite(t)
	for _, listen := range []string{":8080", "0.0.0.0:8080", "[::]:8080", "192.168.1.2:8080", "localhost:8080"} {
		cfg := s.cfg
		cfg.Listen = listen
		if _, err := New(cfg, s.mgr, 7144, nil); err == nil {
			t.Errorf("accepted listen %s", listen)
		}
	}
	for _, origin := range []string{"https://live.example", "http://192.168.1.2:5173", "https://localhost:5173"} {
		cfg := s.cfg
		cfg.Origin = origin
		if _, err := New(cfg, s.mgr, 7144, nil); err == nil {
			t.Errorf("accepted origin %s", origin)
		}
	}
	cfg := s.cfg
	cfg.DevLogin = false
	if _, err := New(cfg, s.mgr, 7144, nil); err == nil {
		t.Fatal("normal mode accepted missing X credentials")
	}
}
