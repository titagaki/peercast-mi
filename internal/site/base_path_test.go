package site

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestBasePathRoutesAndIsolation(t *testing.T) {
	s := testSite(t)
	s.cfg.BasePath = "/mi"
	s.cfg.UIDir = t.TempDir()
	if err := os.Mkdir(filepath.Join(s.cfg.UIDir, "assets"), 0755); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{"index.html": "site page", "assets/app.js": "compiled asset"} {
		if err := os.WriteFile(filepath.Join(s.cfg.UIDir, name), []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{"/mi/", "/mi/broadcast", "/mi/admin", "/mi/channels/0123456789abcdef0123456789abcdef", "/mi/assets/app.js", "/mi/site/api/me"} {
		if w := call(s, "GET", path, "", "", ""); w.Code != 200 {
			t.Errorf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
	for _, path := range []string{"/", "/site/api/me", "/auth/x/start", "/mi/api/1", "/mi/config.toml", "/mismatch/"} {
		if w := call(s, "GET", path, "", "", ""); w.Code != 404 {
			t.Errorf("%s: %d", path, w.Code)
		}
	}
	for _, path := range []string{"/mi", "/mi/watch"} {
		w := call(s, "GET", path, "", "", "")
		if w.Code < 300 || w.Code > 399 || w.Header().Get("Location") != "/mi/" {
			t.Errorf("%s: %d %v", path, w.Code, w.Header())
		}
	}
	ss := addSession(s, "alice", "1")
	if w := call(s, "POST", "/mi/site/api/key", "alice", "", ""); w.Code != http.StatusForbidden {
		t.Fatal(w.Code)
	}
	if w := call(s, "POST", "/mi/site/api/key", "alice", ss.CSRF, ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := call(s, "GET", "/mi/site/stream/0123456789abcdef0123456789abcdef", "", "", ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	for _, next := range []string{"/", "/broadcast", "/mi/../broadcast", "//evil.test", "/mismatch/broadcast", "/mi//evil.test"} {
		if got := s.returnPath(next); got != "/mi/" {
			t.Errorf("%s: %s", next, got)
		}
	}
	if got := s.returnPath("/mi/broadcast"); got != "/mi/broadcast" {
		t.Fatal(got)
	}
}

func TestBasePathValidation(t *testing.T) {
	s := testSite(t)
	for _, base := range []string{"/", "mi", "/mi/", "//mi", "/mi/..", "/mi?x", "/mi%2fother", "/mi\\other"} {
		cfg := s.cfg
		cfg.BasePath = base
		if _, err := New(cfg, s.mgr, 7144, nil); err == nil {
			t.Errorf("accepted %q", base)
		}
	}
	cfg := s.cfg
	cfg.BasePath = "/mi"
	if _, err := New(cfg, s.mgr, 7144, nil); err != nil {
		t.Fatal(err)
	}
}
