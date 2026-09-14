package site

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/titagaki/peercast-mi/internal/channel"
	"golang.org/x/text/encoding/japanese"
)

func TestBoardAddresses(t *testing.T) {
	for _, raw := range []string{"https://jbbs.shitaraba.net/bbs/read.cgi/game/123/456/l50", "http://jbbs.livedoor.jp/game/123/", "https://bbs.jpnkn.com/test/read.cgi/my_board/456/", "https://bbs.jpnkn.com/my_board/"} {
		b, ok := parseBoard(raw)
		if !ok || !strings.HasPrefix(b.subjectURL(), "https://") {
			t.Fatal(raw, b)
		}
	}
	for _, raw := range []string{"http://127.0.0.1/test/read.cgi/b/1/", "https://evil.test/test/read.cgi/b/1/", "https://bbs.jpnkn.com.evil.test/b/", "https://user@bbs.jpnkn.com/b/", "https://bbs.jpnkn.com:444/b/", "file:///etc/passwd", "https://bbs.jpnkn.com/b/../secret/", "https://bbs.jpnkn.com/%2fsecret/"} {
		if _, ok := parseBoard(raw); ok {
			t.Fatal("accepted", raw)
		}
	}
}

// Read-only optional interoperability check, never enabled in ordinary tests.
func TestLiveBoard(t *testing.T) {
	raw := os.Getenv("PEERCAST_TEST_BOARD_URL")
	if raw == "" {
		t.Skip("set PEERCAST_TEST_BOARD_URL for a live BBS read")
	}
	board, ok := parseBoard(raw)
	if !ok {
		t.Fatal("unsupported test board")
	}
	b := newBoardReader()
	ctx, cancel := context.WithTimeout(context.Background(), 11*time.Second)
	defer cancel()
	subject, err := b.read(ctx, board.subjectURL(), board.shitaraba)
	if err != nil {
		t.Fatal(err)
	}
	threads, err := parseSubjects(subject, board.shitaraba)
	if err != nil || len(threads) == 0 {
		t.Fatal("no parsed threads", err)
	}
	thread := board.thread
	if thread == "" {
		thread = threads[0].ID
	}
	dat, err := b.read(ctx, board.datURL(thread), board.shitaraba)
	if err != nil {
		t.Fatal(err)
	}
	comments, _, count, err := parseComments(dat, board.shitaraba)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("threads=%d, comments=%d, latest-number=%d", len(threads), len(comments), count)
}
func TestBoardParsing(t *testing.T) {
	threads, err := parseSubjects("123.cgi,タイトル,続き(32)\n456.cgi,次(0)\n123.cgi,重複(32)\n", true)
	if err != nil || len(threads) != 2 || threads[0].Title != "タイトル,続き" {
		t.Fatal(threads, err)
	}
	threads, err = parseSubjects("123.dat<>題 &amp; 名 (32)\n", false)
	if err != nil || len(threads) != 1 || threads[0].Title != "題 & 名" {
		t.Fatal(threads, err)
	}
	var dat strings.Builder
	for i := 1; i <= 35; i++ {
		fmt.Fprintf(&dat, "名前<>mail<>日付<>本文%d<br />次の行 &lt;b&gt;テキスト&lt;/b&gt;<>タイトル\n", i)
	}
	comments, title, count, err := parseComments(dat.String(), false)
	if err != nil || len(comments) != 30 || count != 35 || comments[0].No != 35 || comments[29].No != 6 || title != "タイトル" || !strings.Contains(comments[0].Body, "\n次の行 <b>テキスト</b>") {
		t.Fatal(comments, title, count, err)
	}
	comments, title, count, err = parseComments("10<>名前<><>日時<><a href='evil'>本文</a><br>次<>題\n12<>名前<><>日時<>末尾<>\n", true)
	if err != nil || count != 12 || comments[0].No != 12 || comments[1].Body != "本文\n次" || title != "題" {
		t.Fatal(comments, title, count, err)
	}
	if _, _, _, err := parseComments("bad", false); err == nil {
		t.Fatal("malformed dat accepted")
	}
}
func TestBoardCacheEncodingAndBounds(t *testing.T) {
	for _, shitaraba := range []bool{true, false} {
		b := newBoardReader()
		var calls atomic.Int32
		enc := japanese.ShiftJIS
		if shitaraba {
			enc = japanese.EUCJP
		}
		encoded, _ := enc.NewEncoder().String("日本語<>テスト")
		b.client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
			calls.Add(1)
			if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
				t.Error("credentials leaked")
			}
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(encoded))}, nil
		})
		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				data, err := b.read(context.Background(), "https://bbs.jpnkn.com/b/subject.txt", shitaraba)
				if err != nil || data != "日本語<>テスト" {
					t.Error(data, err)
				}
			}()
		}
		wg.Wait()
		if calls.Load() != 1 {
			t.Fatal("cache stampede", calls.Load())
		}
	}
	b := newBoardReader()
	b.client.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(strings.Repeat("a", (2<<20)+1)))}, nil
	})
	if _, err := b.fetch(context.Background(), "https://bbs.jpnkn.com/b/subject.txt", false); err == nil {
		t.Fatal("unbounded response")
	}
	for i := 0; i < 64; i++ {
		b.cache[fmt.Sprint(i)] = &boardCacheEntry{expires: time.Now().Add(time.Hour)}
	}
	if _, err := b.read(context.Background(), "new", false); err == nil {
		t.Fatal("unbounded cache")
	}
}

func TestAuthenticatedBoardEndpoint(t *testing.T) {
	s := testSite(t)
	addSession(s, "alice", "1")
	if err := s.mgr.IssueStreamKey("a", "key"); err != nil {
		t.Fatal(err)
	}
	ch, err := s.mgr.Broadcast("key", channel.ChannelInfo{Name: "test", URL: "https://bbs.jpnkn.com/test/read.cgi/board/123/"}, channel.TrackInfo{})
	if err != nil {
		t.Fatal(err)
	}
	path := "/site/api/channels/" + hex.EncodeToString(ch.ID[:]) + "/comments"
	var calls atomic.Int32
	s.boards.client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		body := ""
		switch r.URL.String() {
		case "https://bbs.jpnkn.com/board/subject.txt":
			body = "123.dat<>題 (1)\n456.dat<>次 (1)\n"
		case "https://bbs.jpnkn.com/board/dat/123.dat":
			body = "名無し<><>日付<>こんにちは<br>世界<>題\n"
		case "https://bbs.jpnkn.com/board/dat/456.dat":
			body = "名無し<><>日付<>次のコメント<>次\n"
		default:
			t.Error("unexpected target", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	if w := call(s, "GET", path, "", "", ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if calls.Load() != 0 {
		t.Fatal("unauthenticated fetch")
	}
	w := call(s, "GET", path, "alice", "", "")
	var result boardView
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.ThreadID != "123" || len(result.Comments) != 1 || result.Comments[0].Body != "こんにちは\n世界" {
		t.Fatal(w.Code, w.Body.String())
	}
	w = call(s, "GET", path+"?thread=456", "alice", "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "次のコメント") {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, q := range []string{"?url=http://127.0.0.1", "?thread=../../foo", "?thread=123&thread=456", "?thread=%zz"} {
		if w := call(s, "GET", path+q, "alice", "", ""); w.Code != 400 {
			t.Fatal(q, w.Code)
		}
	}
	if w := call(s, "GET", path+"?thread=789", "alice", "", ""); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if calls.Load() != 3 {
		t.Fatal("unexpected fetch count", calls.Load())
	}
}

func TestSitePageRoutesAndReturnPath(t *testing.T) {
	s := testSite(t)
	s.cfg.UIDir = t.TempDir()
	if err := os.WriteFile(filepath.Join(s.cfg.UIDir, "index.html"), []byte("<!doctype html><title>UI</title>"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/", "/channels/0123456789abcdef0123456789abcdef", "/broadcast", "/admin", "/admin/"} {
		w := call(s, "GET", path, "", "", "")
		if w.Code != 200 || !strings.Contains(w.Body.String(), "<title>UI") {
			t.Fatal(path, w.Code)
		}
	}
	if w := call(s, "GET", "/watch", "", "", ""); w.Code != 302 || w.Header().Get("Location") != "/" {
		t.Fatal(w.Code)
	}
	for _, path := range []string{"/channels/bad", "/api/1", "/admin/secrets"} {
		if w := call(s, "GET", path, "", "", ""); w.Code != 404 {
			t.Fatal(path, w.Code)
		}
	}
	for _, path := range []string{"https://evil.test", "//evil.test", "/admin", "/channels/../../evil"} {
		if safeReturnPath(path) != "/" {
			t.Fatal(path)
		}
	}
	if safeReturnPath("/broadcast") != "/broadcast" {
		t.Fatal("broadcast return")
	}
	r := httptest.NewRequest("GET", "/auth/x/start?next=/channels/0123456789abcdef0123456789abcdef", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 302 {
		t.Fatal(w.Code)
	}
	for _, f := range s.flows {
		if f.Next != "/channels/0123456789abcdef0123456789abcdef" {
			t.Fatal(f.Next)
		}
	}
}
