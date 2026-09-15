package site

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestBroadcastCreatorUsesRequestIP(t *testing.T) {
	for _, tc := range []struct{ name, remote, xff, want string }{
		{"direct", "153.229.89.75:1234", "203.0.113.9", "153.229.89.75"},
		{"proxy", "172.20.0.2:1234", "153.229.89.75", "153.229.89.75"},
		{"spoofed prefix", "172.20.0.2:1234", "203.0.113.9, 153.229.89.75", "153.229.89.75"},
		{"multiple proxies", "172.20.0.2:1234", "153.229.89.75, 127.0.0.1", "153.229.89.75"},
		{"ipv6", "[2001:db8::123]:1234", "", "2001:db8::123"},
		{"mapped ipv4", "[::ffff:153.229.89.75]:1234", "", "153.229.89.75"},
		{"missing header", "172.20.0.2:1234", "", "172.20.0.2"},
		{"malformed hop", "172.20.0.2:1234", "153.229.89.75, garbage", "172.20.0.2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testSite(t)
			ss := addSession(s, "alice", "1")
			s.trustedProxies = []netip.Prefix{netip.MustParsePrefix("172.16.0.0/12"), netip.MustParsePrefix("127.0.0.1/32")}
			call(s, "POST", "/site/api/key", "alice", ss.CSRF, "")
			r := httptest.NewRequest("POST", "/site/api/broadcast", strings.NewReader(`{"name":"Live"}`))
			r.RemoteAddr = tc.remote
			r.Header.Set("X-Forwarded-For", tc.xff)
			r.Header.Set("X-Real-IP", "203.0.113.99")
			r.Header.Set("Origin", s.cfg.Origin)
			r.Header.Set("X-CSRF-Token", ss.CSRF)
			r.AddCookie(&http.Cookie{Name: sessionCookie, Value: "alice"})
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
			ch, ok := s.mgr.GetByStreamKey(s.key(ss))
			if !ok {
				t.Fatal("missing channel")
			}
			if got := ch.Track().ToPCP().Creator; got != tc.want+" via PecaMI" {
				t.Fatal(got)
			}
			history, err := s.readHistory(account(ss))
			if err != nil || len(history) != 1 {
				t.Fatal(history, err)
			}
		})
	}
}
func TestTrustedProxyConfiguration(t *testing.T) {
	s := testSite(t)
	cfg := s.cfg
	cfg.TrustedProxies = []string{"172.16.0.0/12"}
	next, err := New(cfg, s.mgr, 7144, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "172.20.0.2:1234"
	r.Header.Set("X-Forwarded-For", "153.229.89.75")
	if next.broadcastClientIP(r) != "153.229.89.75" {
		t.Fatal("configured proxy not used")
	}
	if s.broadcastClientIP(r) != "172.20.0.2" {
		t.Fatal("unconfigured proxy trusted")
	}
	cfg.TrustedProxies = []string{"not-a-cidr"}
	if _, err := New(cfg, s.mgr, 7144, nil); err == nil {
		t.Fatal("invalid CIDR accepted")
	}
}
