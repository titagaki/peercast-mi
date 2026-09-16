package site

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/titagaki/peercast-mi/internal/audit"
	"github.com/titagaki/peercast-mi/internal/config"
	"github.com/titagaki/peercast-mi/internal/jsonrpc"
	"github.com/titagaki/peercast-pcp/pcp"
)

type auditCapture struct {
	mu     sync.Mutex
	events []audit.Event
}

func (c *auditCapture) Emit(e audit.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
}
func TestAuditLoginBroadcastAdminAndSecrets(t *testing.T) {
	s := testSite(t)
	log := &auditCapture{}
	s.mgr.Audit = log
	req := httptest.NewRequest("GET", "/auth/x/callback", nil)
	req.RemoteAddr = "[2001:db8::123]:5000"
	w := httptest.NewRecorder()
	if !s.startSession(w, req, user{ID: "123", Name: "配信者"}) {
		t.Fatal(w.Body.String())
	}
	cookie := w.Result().Cookies()[0]
	ss := s.sessions[cookie.Value]
	if len(ss.AuditRef) != 32 || ss.AuditRef == cookie.Value || ss.AuditRef == ss.CSRF {
		t.Fatal("unsafe session reference")
	}
	if w = call(s, "POST", "/site/api/key", cookie.Value, ss.CSRF, ""); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	secret := s.key(ss)
	if w = call(s, "POST", "/site/api/broadcast", cookie.Value, ss.CSRF, `{"name":"テスト","genre":"ゲーム","comment":"コメント"}`); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	ch, ok := s.mgr.GetByStreamKey(secret)
	if !ok {
		t.Fatal("missing channel")
	}
	admin := addSession(s, "admin-cookie", "999")
	admin.AuditRef = audit.ID()
	s.adminIDs["999"] = true
	api := jsonrpc.New(pcp.GnuID{}, s.mgr, &config.Config{}, nil)
	s.SetAdminHandler(api.Handler())
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"stopChannel","params":["%x"]}`, ch.ID[:])
	stop := httptest.NewRequest("POST", "/admin/api/1", strings.NewReader(body))
	stop.RemoteAddr = "192.0.2.42:5"
	stop.Header.Set("Content-Type", "application/json")
	stop.Header.Set("Origin", s.cfg.Origin)
	stop.Header.Set("X-CSRF-Token", admin.CSRF)
	stop.AddCookie(&http.Cookie{Name: sessionCookie, Value: "admin-cookie"})
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, stop)
	if w.Code != 200 || strings.Contains(w.Body.String(), "error") {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = call(s, "POST", "/site/api/logout", cookie.Value, ss.CSRF, ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var end, create *audit.Event
	for i := range log.events {
		e := &log.events[i]
		if e.Type == "broadcast.create" {
			create = e
		}
		if e.Type == "broadcast.end" {
			end = e
		}
	}
	if end == nil || end.Actor.Account != "site:x:999" || end.Owner != "site:x:123" || end.Actor.IP != "192.0.2.42" || end.Reason != "admin_stop" {
		t.Fatal(end)
	}
	if create == nil || create.Payload.Broadcast.Settings.InputGenre == nil || *create.Payload.Broadcast.Settings.InputGenre != "ゲーム" || create.Payload.Broadcast.Settings.Genre != "ypゲーム" {
		t.Fatal(create)
	}
	raw, _ := json.Marshal(log.events)
	for _, forbidden := range []string{secret, cookie.Value, ss.CSRF, admin.CSRF, "test-secret"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatal("credential entered audit payload")
		}
	}
	if w = call(s, "GET", "/site/api/audit/status", "admin-cookie", "", ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	ordinary := addSession(s, "ordinary", "3")
	if w = call(s, "GET", "/site/api/audit/status", "ordinary", ordinary.CSRF, ""); w.Code != 403 {
		t.Fatal("status exposed", w.Code)
	}
}
func TestAuditHistoryRollbackAndCancelledLogin(t *testing.T) {
	s := testSite(t)
	log := &auditCapture{}
	s.mgr.Audit = log
	ss := addSession(s, "owner", "123")
	if w := call(s, "POST", "/site/api/key", "owner", ss.CSRF, ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	s.cfg.BroadcastHistoryDir = filepath.Join("/proc", "peercast-audit-test", "history")
	if w := call(s, "POST", "/site/api/broadcast", "owner", ss.CSRF, `{"name":"test"}`); w.Code != 500 {
		t.Fatal(w.Code, w.Body.String())
	}
	n := 0
	for _, e := range log.events {
		if e.Type == "broadcast.create" {
			n++
			if e.Outcome != "failure" || e.Reason != "setup_rollback" {
				t.Fatal(e)
			}
		}
	}
	if n != 1 {
		t.Fatal(n)
	}
	s.flows["flow-secret"] = flow{State: "state-secret", Expires: time.Now().Add(time.Minute)}
	req := httptest.NewRequest("GET", "/auth/x/callback?state=state-secret&error=access_denied", nil)
	req.AddCookie(&http.Cookie{Name: flowCookie, Value: "flow-secret"})
	w := httptest.NewRecorder()
	s.callback(w, req)
	e := log.events[len(log.events)-1]
	if e.Type != "auth.login" || e.Outcome != "cancelled" || e.Actor.Account != "" {
		t.Fatal(e)
	}
}
