package site

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestBroadcastHistoryPersistenceOwnershipAndLimit(t *testing.T) {
	s := testSite(t)
	a := addSession(s, "alice", "1")
	addSession(s, "bob", "2")
	call(s, "POST", "/site/api/key", "alice", a.CSRF, "")
	for i := 0; i < 12; i++ {
		body := fmt.Sprintf(`{"name":"Live %d","genre":" Music ","description":"Details","comment":"Hello","contactUrl":"https://bbs.jpnkn.com/board/"}`, i)
		w := call(s, "POST", "/site/api/broadcast", "alice", a.CSRF, body)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		call(s, "DELETE", "/site/api/broadcast", "alice", a.CSRF, "")
	}
	history, err := s.readHistory(account(a))
	if err != nil || len(history) != 10 || history[0].Name != "Live 11" || history[9].Name != "Live 2" || history[0].Genre != "Music" || history[0].CreatedAt.IsZero() {
		t.Fatal(history, err)
	}
	if info, err := os.Stat(s.historyPath(account(a))); err != nil || info.Mode().Perm() != 0600 {
		t.Fatal(info, err)
	}
	// Recreating the server and issuing a new key must keep X-account history.
	next, err := New(s.cfg, s.mgr, 7144, nil)
	if err != nil {
		t.Fatal(err)
	}
	a2 := addSession(next, "alice2", "1")
	addSession(next, "bob2", "2")
	call(next, "POST", "/site/api/key", "alice2", a2.CSRF, "")
	for _, tc := range []struct {
		cookie string
		want   int
	}{{"alice2", 10}, {"bob2", 0}} {
		w := call(next, "GET", "/site/api/broadcast", tc.cookie, "", "")
		var v struct {
			History []broadcastHistoryEntry `json:"history"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &v) != nil || len(v.History) != tc.want {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
func TestBroadcastHistoryFailureDoesNotLeaveChannelOrOverwriteHistory(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		t.Run(fmt.Sprint(corrupt), func(t *testing.T) {
			s := testSite(t)
			a := addSession(s, "alice", "1")
			call(s, "POST", "/site/api/key", "alice", a.CSRF, "")
			if corrupt {
				if err := os.MkdirAll(s.cfg.BroadcastHistoryDir, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(s.historyPath(account(a)), []byte("broken"), 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				// Read sees ENOENT, but creating the missing parent under /proc fails.
				s.cfg.BroadcastHistoryDir = filepath.Join("/proc", "peercast-history-test", "history")
			}
			w := call(s, "POST", "/site/api/broadcast", "alice", a.CSRF, `{"name":"Live"}`)
			if w.Code != 500 {
				t.Fatal(w.Code, w.Body.String())
			}
			if _, ok := s.mgr.GetByStreamKey(s.key(a)); ok {
				t.Fatal("failed save left active channel")
			}
			if corrupt {
				data, _ := os.ReadFile(s.historyPath(account(a)))
				if string(data) != "broken" {
					t.Fatal("overwrote corrupt history")
				}
			}
		})
	}
}
