package site

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestBroadcastBoardLatestUnfilledAndArchivedThread(t *testing.T) {
	for _, tc := range []struct{ contact, settings, subject, dat string }{
		{"https://bbs.jpnkn.com/test/read.cgi/imai/100/", "BBS_TITLE=いまいch\nBBS_THREAD_STOP=1001", "200.dat<>新スレ (2)\n300.dat<>満員 (1001)\n150.dat<>古い (5)", "名前<>mail<>date<>body<>いまいch 212\n"},
		{"https://jbbs.shitaraba.net/bbs/read.cgi/game/123/100/", "BBS_TITLE=いまいch\nBBS_THREAD_STOP=1001", "200.cgi,新スレ (2)\n300.cgi,満員 (1001)\n150.cgi,古い (5)", "1001<>名前<>mail<>date<>body<>いまいch 212\n"},
	} {
		t.Run(tc.contact, func(t *testing.T) {
			s := testSite(t)
			addSession(s, "alice", "1")
			b, _ := parseBoard(tc.contact)
			s.boards.client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
				var body string
				switch r.URL.String() {
				case b.settingsURL():
					body = tc.settings
				case b.subjectURL():
					body = tc.subject
				case b.datURL("100"):
					body = tc.dat
				default:
					t.Errorf("unexpected fetch %s", r.URL)
					return nil, io.EOF
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/plain; charset=utf-8"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			w := call(s, "GET", "/site/api/broadcast/board?url="+url.QueryEscape(tc.contact), "alice", "", "")
			var v broadcastBoardView
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &v) != nil {
				t.Fatal(w.Code, w.Body.String())
			}
			if v.BoardTitle != "いまいch" || v.Thread == nil || v.Thread.Title != "いまいch 212" || v.LatestThread == nil || v.LatestThread.ID != "200" || v.LatestThreadURL != b.threadURL("200") {
				t.Fatal(w.Body.String())
			}
		})
	}
}
func TestBroadcastBoardRejectsUnsafeURLsAndRequiresLogin(t *testing.T) {
	s := testSite(t)
	addSession(s, "alice", "1")
	s.boards.client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected fetch %s", r.URL)
		return nil, io.EOF
	})
	if w := call(s, "GET", "/site/api/broadcast/board?url=https://bbs.jpnkn.com/board/", "", "", ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	for _, raw := range []string{"http://127.0.0.1/", "https://bbs.jpnkn.com.evil.example/board/", "https://user@bbs.jpnkn.com/board/", "https://bbs.jpnkn.com:443/board/", "https://bbs.jpnkn.com/../private"} {
		w := call(s, "GET", "/site/api/broadcast/board?url="+url.QueryEscape(raw), "alice", "", "")
		var v broadcastBoardView
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &v) != nil || v.Supported {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if w := call(s, "GET", "/site/api/broadcast/board?url=a&url=b", "alice", "", ""); w.Code != 400 {
		t.Fatal(w.Code)
	}
}
func TestBroadcastBoardAllFullAndFetchFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		s := testSite(t)
		addSession(s, "alice", "1")
		s.boards.client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
			if fail {
				return nil, io.EOF
			}
			body := "200.dat<>満員 (1000)\n"
			if strings.HasSuffix(r.URL.Path, "SETTING.TXT") {
				body = "BBS_TITLE=Board\n"
			}
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
		})
		w := call(s, "GET", "/site/api/broadcast/board?url=https://bbs.jpnkn.com/board/", "alice", "", "")
		if fail {
			if w.Code != 502 {
				t.Fatal(w.Code)
			}
			continue
		}
		var v broadcastBoardView
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &v) != nil || v.LatestThread != nil {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
