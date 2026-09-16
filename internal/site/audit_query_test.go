package site

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/titagaki/peercast-mi/internal/audit"
)

type historyReader struct {
	calls   int
	filter  audit.Filter
	inputID string
	err     error
}

func (r *historyReader) Events(ctx context.Context, f audit.Filter) (audit.Page[audit.EventRecord], error) {
	r.calls++
	r.filter = f
	if _, ok := ctx.Deadline(); !ok {
		panic("missing deadline")
	}
	return audit.Page[audit.EventRecord]{Items: []audit.EventRecord{}}, r.err
}
func (r *historyReader) Broadcasts(_ context.Context, f audit.Filter) (audit.Page[audit.Broadcast], error) {
	r.calls++
	r.filter = f
	return audit.Page[audit.Broadcast]{Items: []audit.Broadcast{}}, r.err
}
func (r *historyReader) Inputs(_ context.Context, id string, f audit.Filter) (audit.Page[audit.Input], error) {
	r.calls++
	r.filter = f
	r.inputID = id
	return audit.Page[audit.Input]{Items: []audit.Input{}}, r.err
}
func TestAuditHistoryAuthorizationAndValidation(t *testing.T) {
	s := testSite(t)
	reader := &historyReader{}
	s.SetAuditReader(reader)
	addSession(s, "owner", "1")
	s.adminIDs["2"] = true
	addSession(s, "admin", "2")
	id := strings.Repeat("a", 32)
	for _, path := range []string{"/site/api/audit/events", "/site/api/audit/broadcasts", "/site/api/audit/broadcasts/" + id + "/inputs"} {
		if w := call(s, "GET", path, "", "", ""); w.Code != 401 {
			t.Fatal(w.Code)
		}
		if w := call(s, "GET", path, "owner", "", ""); w.Code != 403 {
			t.Fatal(w.Code)
		}
		if reader.calls != 0 {
			t.Fatal("unauthorized DB read")
		}
	}
	if w := call(s, "GET", "/site/api/audit/events?limit=101", "admin", "", ""); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := call(s, "GET", "/site/api/audit/broadcasts/not-an-id/inputs", "admin", "", ""); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if reader.calls != 0 {
		t.Fatal("invalid query reached DB")
	}
	for _, path := range []string{"/site/api/audit/events?actor=site:x:1&limit=25", "/site/api/audit/broadcasts?status=ended", "/site/api/audit/broadcasts/" + id + "/inputs"} {
		w := call(s, "GET", path, "admin", "", "")
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Body.String(), `"items":[]`) {
			t.Fatal(w.Code, w.Header(), w.Body.String())
		}
	}
	if reader.calls != 3 || reader.inputID != id {
		t.Fatal(reader)
	}
	reader.err = errors.New("password=secret SQL SELECT private_column")
	w := call(s, "GET", "/site/api/audit/events", "admin", "", "")
	if w.Code != 503 || strings.Contains(w.Body.String(), "secret") {
		t.Fatal(w.Code, w.Body.String())
	}
	s.cfg.DevLogin = true
	// Direct handler call isolates the admin check from development host restrictions.
	w = httptest.NewRecorder()
	s.auditEvents(w, httptest.NewRequest("GET", "/site/api/audit/events", nil), s.sessions["admin"])
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}
func TestAuditHistoryUnavailableAndBusy(t *testing.T) {
	s := testSite(t)
	s.adminIDs["1"] = true
	addSession(s, "admin", "1")
	if w := call(s, "GET", "/site/api/audit/events", "admin", "", ""); w.Code != 503 {
		t.Fatal(w.Code)
	}
	r := &historyReader{}
	s.SetAuditReader(r)
	s.auditReadSlots <- struct{}{}
	s.auditReadSlots <- struct{}{}
	if w := call(s, "GET", "/site/api/audit/events", "admin", "", ""); w.Code != 503 || r.calls != 0 {
		t.Fatal(w.Code, r.calls)
	}
	<-s.auditReadSlots
	<-s.auditReadSlots
	if w := call(s, "GET", "/site/api/audit/events", "admin", "", ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
}
