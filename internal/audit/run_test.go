package audit

import (
	"sync"
	"testing"
	"time"
)

type capture struct {
	mu     sync.Mutex
	events []Event
}

func (s *capture) Emit(e Event) { s.mu.Lock(); defer s.mu.Unlock(); s.events = append(s.events, e) }
func TestRunLifecycle(t *testing.T) {
	s := &capture{}
	a := Actor{Account: "site:x:owner", Name: "配信者", Source: "site", IP: "192.0.2.1:44"}
	r := NewRun(s, ID(), a.Account, a, Settings{Name: "日本語🎥", Genre: "ypゲーム"}, true)
	if len(s.events) != 0 {
		t.Fatal("premature create")
	}
	r.Commit()
	r.Commit()
	conn := ID()
	r.Media(conn, "[2001:db8::1]:5000")
	r.Media(conn, "[2001:db8::1]:5000")
	r.mu.Lock()
	r.published[conn] = time.Now().Add(-time.Minute)
	r.mu.Unlock()
	r.Media(conn, "[2001:db8::1]:5000")
	admin := Actor{Account: "site:x:admin", Source: "site"}
	r.Metadata(admin, Settings{Name: "変更後", Genre: "ypゲーム"})
	r.End(admin, "admin_stop")
	r.End(admin, "admin_stop")
	expected := []string{"broadcast.create", "input.start", "input.progress", "broadcast.metadata", "input.end", "broadcast.end"}
	if len(s.events) != len(expected) {
		t.Fatal(s.events)
	}
	for i, e := range s.events {
		if e.Type != expected[i] {
			t.Fatal(i, e.Type)
		}
		if e.Payload.Broadcast.Revision != uint64(i+1) {
			t.Fatal("revision")
		}
	}
	end := s.events[len(s.events)-1]
	b := end.Payload.Broadcast
	if b.FirstMedia == nil || b.LastMedia == nil || b.Ended == nil || b.Settings.Name != "日本語🎥" || b.Actor.Account != a.Account || end.Actor.Account != admin.Account || end.Owner != a.Account {
		t.Fatal(end)
	}
	if s.events[1].Payload.Input.Ended != nil {
		t.Fatal("snapshot mutated after enqueue")
	}
}
func TestRollbackAndDistinctRuns(t *testing.T) {
	s := &capture{}
	ch := ID()
	a := Actor{Source: "admin", Account: "admin:basic", Name: "管理者"}
	one := NewRun(s, ch, "site:x:owner", a, Settings{}, true)
	one.End(a, "setup_rollback")
	one.Commit()
	two := NewRun(s, ch, "site:x:owner", a, Settings{}, false)
	if one.Snapshot().ID == two.Snapshot().ID || one.Snapshot().OwnerName != "" {
		t.Fatal("identity conflated")
	}
	if len(s.events) != 2 || s.events[0].Outcome != "failure" || s.events[0].Reason != "setup_rollback" {
		t.Fatal(s.events)
	}
}
