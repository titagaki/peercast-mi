package site

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBroadcastMetadataFields(t *testing.T) {
	s := testSite(t)
	ss := addSession(s, "alice", "123")
	call(s, "POST", "/site/api/key", "alice", ss.CSRF, "")
	w := call(s, "POST", "/site/api/broadcast", "alice", ss.CSRF, `{"name":"Live","genre":"Music","description":"Desc","comment":"Hello","contactUrl":"https://bbs.jpnkn.com/board/","bitrate":3000}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	ch, ok := s.mgr.GetByStreamKey(s.key(ss))
	if !ok {
		t.Fatal("no broadcast")
	}
	info := ch.Info()
	if info.Comment != "Hello" || info.URL != "https://bbs.jpnkn.com/board/" || info.Bitrate != 3000 {
		t.Fatal(info)
	}
	var result channelView
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Comment != info.Comment || result.ContactURL != info.URL {
		t.Fatal(result)
	}
	call(s, "DELETE", "/site/api/broadcast", "alice", ss.CSRF, "")
	w = call(s, "POST", "/site/api/broadcast", "alice", ss.CSRF, `{"name":"Minimal"}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	ch, _ = s.mgr.GetByStreamKey(s.key(ss))
	if ch.Info().Bitrate != 0 {
		t.Fatal(ch.Info())
	}
}

func TestBroadcastMetadataRejectsInvalidValues(t *testing.T) {
	s := testSite(t)
	ss := addSession(s, "alice", "123")
	call(s, "POST", "/site/api/key", "alice", ss.CSRF, "")
	for _, extra := range []string{
		`"bitrate":-1`, `"bitrate":2147483648`, `"bitrate":1.5`,
		`"contactUrl":"javascript:alert(1)"`, `"contactUrl":"/relative"`,
		`"contactUrl":"https://user:pass@example.com/"`,
		`"comment":"` + strings.Repeat("x", 2049) + `"`,
		`"contactUrl":"https://example.com/` + strings.Repeat("x", 2048) + `"`,
	} {
		w := call(s, "POST", "/site/api/broadcast", "alice", ss.CSRF, `{"name":"Live",`+extra+`}`)
		if w.Code != 400 {
			t.Errorf("invalid metadata accepted: %d", w.Code)
		}
		if _, ok := s.mgr.GetByStreamKey(s.key(ss)); ok {
			t.Fatal("invalid request created a channel")
		}
	}
}

func TestBroadcastAddsPublicationGenrePrefix(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"", "yp"}, {"ゲーム", "ypゲーム"}, {" Music ", "ypMusic"}, {"ypゲーム", "ypゲーム"}, {"yp?@@@game", "yp?@@@game"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			s := testSite(t)
			ss := addSession(s, "alice", "123")
			call(s, "POST", "/site/api/key", "alice", ss.CSRF, "")
			body, _ := json.Marshal(map[string]string{"name": "Live", "genre": tc.input})
			w := call(s, "POST", "/site/api/broadcast", "alice", ss.CSRF, string(body))
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
			ch, ok := s.mgr.GetByStreamKey(s.key(ss))
			if !ok {
				t.Fatal("missing channel")
			}
			if got := ch.Info().ToPCP().Genre; got != tc.want {
				t.Fatalf("PCP genre=%q want %q", got, tc.want)
			}
		})
	}
}
