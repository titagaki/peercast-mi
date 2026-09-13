package servent

import (
	"bufio"
	"encoding/hex"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/titagaki/peercast-pcp/pcp"

	"github.com/titagaki/peercast-mi/internal/channel"
	"github.com/titagaki/peercast-mi/internal/version"
)

// writePCPMagic は "pcp\n" ハンドシェイク冒頭の 12 バイト (tag+length+version) を書き込む。
func writePCPMagic(conn net.Conn) error {
	return pcp.NewIntAtom(pcp.NewID4("pcp\n"), version.PCPVersion).Write(conn)
}

// pingResult は client goroutine が収集した応答を保持する。
type pingResult struct {
	writeErr error
	oleh     *pcp.Atom
	quit     *pcp.Atom
	readErr  error
}

// runPingClient は net.Pipe の client 側で「送信 → 受信」を 1 goroutine 内で行い
// 結果をチャネルに返す。net.Pipe はブロッキング同期なので、送信と受信を同一 goroutine に
// まとめることでデッドロックを回避する。
func runPingClient(client net.Conn, clientID, sessionID pcp.GnuID) <-chan pingResult {
	ch := make(chan pingResult, 1)
	go func() {
		var r pingResult
		defer client.Close()

		if err := writePCPMagic(client); err != nil {
			r.writeErr = err
			ch <- r
			return
		}
		helo := pcp.NewParentAtom(pcp.PCPHelo,
			pcp.NewIDAtom(pcp.PCPHeloSessionID, clientID),
			pcp.NewIntAtom(pcp.PCPHeloVersion, version.PCPVersion),
		)
		if err := helo.Write(client); err != nil {
			r.writeErr = err
			ch <- r
			return
		}

		// handlePing がレスポンスを書き込んでいる間、ここで読み取る。
		oleh, err := pcp.ReadAtom(client)
		if err != nil {
			r.readErr = err
			ch <- r
			return
		}
		r.oleh = oleh

		quit, err := pcp.ReadAtom(client)
		if err != nil {
			r.readErr = err
			ch <- r
			return
		}
		r.quit = quit
		ch <- r
	}()
	return ch
}

// TestHandlePing_SendsOlehAndQuit は handlePing が oleh(sid) + quit を返すことを確認する。
func TestHandlePing_SendsOlehAndQuit(t *testing.T) {
	client, server := net.Pipe()

	sessionID := pcp.GnuID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	clientID := pcp.GnuID{17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}

	resultCh := runPingClient(client, clientID, sessionID)

	br := bufio.NewReader(server)
	handlePing(server, br, sessionID)

	r := <-resultCh

	if r.writeErr != nil {
		t.Fatalf("client write error: %v", r.writeErr)
	}
	if r.readErr != nil {
		t.Fatalf("client read error: %v", r.readErr)
	}

	if r.oleh == nil || r.oleh.Tag != pcp.PCPOleh {
		t.Fatalf("expected oleh, got %v", r.oleh)
	}
	sidAtom := r.oleh.FindChild(pcp.PCPHeloSessionID)
	if sidAtom == nil {
		t.Fatal("oleh has no sid child")
	}
	sid, err := sidAtom.GetID()
	if err != nil {
		t.Fatalf("get sid: %v", err)
	}
	if sid != sessionID {
		t.Errorf("oleh.sid: got %v, want %v", sid, sessionID)
	}

	if r.quit == nil || r.quit.Tag != pcp.PCPQuit {
		t.Fatalf("expected quit, got %v", r.quit)
	}
}

// TestHandlePing_BadMagic は不正なマジックバイト (12バイト未満) で接続が
// 静かに閉じられることを確認する (パニックしない)。
func TestHandlePing_BadMagic(t *testing.T) {
	client, server := net.Pipe()

	go func() {
		client.Write([]byte{0x00, 0x01})
		client.Close()
	}()

	br := bufio.NewReader(server)
	handlePing(server, br, pcp.GnuID{})
}

// TestHandlePing_InvalidHelo は pcp\n 後に helo 以外のアトムが来た場合に
// 静かに閉じられることを確認する (パニックしない)。
func TestHandlePing_InvalidHelo(t *testing.T) {
	client, server := net.Pipe()

	go func() {
		defer client.Close()
		writePCPMagic(client)
		pcp.NewIntAtom(pcp.PCPQuit, pcp.PCPErrorQuit+pcp.PCPErrorShutdown).Write(client)
	}()

	br := bufio.NewReader(server)
	handlePing(server, br, pcp.GnuID{})
}

type notFoundStore struct{}

func (notFoundStore) GetByID(pcp.GnuID) (*channel.Channel, bool) { return nil, false }
func (notFoundStore) TotalRelays() int                           { return 0 }
func (notFoundStore) TotalSendRate() int64                       { return 0 }

// httpGet は net.Pipe の client 側で 1 リクエストを送り、レスポンス全体を返す。
func httpGet(t *testing.T, client net.Conn, path string) (status string, body string) {
	t.Helper()
	respCh := make(chan string, 1)
	go func() {
		defer client.Close()
		io.WriteString(client, "GET "+path+" HTTP/1.0\r\nHost: localhost\r\n\r\n")
		b, _ := io.ReadAll(client)
		respCh <- string(b)
	}()
	select {
	case resp := <-respCh:
		head, rest, _ := strings.Cut(resp, "\r\n")
		_, body, _ = strings.Cut(rest, "\r\n\r\n")
		return head, body
	case <-time.After(5 * time.Second):
		t.Fatal("no response")
		return "", ""
	}
}

// TestHandlePLS_CallsOnDemandRelayWithoutTip は tip のない /pls/ でも OnDemandRelay が
// 空の tip で呼ばれる (tracker 探索は実装側に委ねる) ことを確認する。
func TestHandlePLS_CallsOnDemandRelayWithoutTip(t *testing.T) {
	for _, tc := range []struct{ query, wantTip string }{
		{"", ""},
		{"?tip=203.0.113.9:7144", "203.0.113.9:7144"},
	} {
		l := NewListener(pcp.GnuID{}, notFoundStore{}, 7144, 0, 0, 0, 0)
		var gotTip string
		called := false
		l.OnDemandRelay = func(id pcp.GnuID, tip string) (*channel.Channel, error) {
			called = true
			gotTip = tip
			return channel.New(id, pcp.GnuID{}, 0), nil
		}
		server, client := net.Pipe()
		done := make(chan struct{})
		go func() {
			l.handlePLS(newCountingConn(server), bufio.NewReader(server))
			close(done)
		}()
		status, body := httpGet(t, client, "/pls/000102030405060708090a0b0c0d0e0f"+tc.query)
		<-done
		if !called || gotTip != tc.wantTip {
			t.Fatalf("query %q: called=%v tip=%q, want tip %q", tc.query, called, gotTip, tc.wantTip)
		}
		if status != "HTTP/1.0 200 OK" || !strings.Contains(body, "http://localhost/stream/000102030405060708090a0b0c0d0e0f") {
			t.Fatalf("query %q: status %q body %q", tc.query, status, body)
		}
	}
}

// TestHandlePLS_Errors は不正な要求とチャンネル未登録時のステータスを確認する。
func TestHandlePLS_Errors(t *testing.T) {
	for _, tc := range []struct {
		path       string
		wantStatus string
	}{
		{"/pls/xyz", "HTTP/1.0 400 Bad Request"},
		{"/pls/000102030405060708090a0b0c0d0e0f?tip=nohost", "HTTP/1.0 400 Bad Request"},
		{"/pls/000102030405060708090a0b0c0d0e0f", "HTTP/1.0 404 Not Found"},
	} {
		l := NewListener(pcp.GnuID{}, notFoundStore{}, 7144, 0, 0, 0, 0)
		server, client := net.Pipe()
		go l.handlePLS(newCountingConn(server), bufio.NewReader(server))
		status, _ := httpGet(t, client, tc.path)
		if status != tc.wantStatus {
			t.Fatalf("%s: status %q, want %q", tc.path, status, tc.wantStatus)
		}
	}
}

func TestParseChannelIDPath(t *testing.T) {
	const id = "000102030405060708090a0b0c0d0e0f"
	for _, tc := range []struct {
		path string
		ok   bool
	}{
		{"/stream/" + id, true},
		{"/stream/" + id + ".flv", true},
		{"/stream/" + id + ".m3u8", true},
		{"/stream/" + id + "x", false},
		{"/stream/" + id + ".", false},
		{"/stream/" + id + ".flv/x", false},
		{"/stream/" + id + "/x", false},
		{"/stream/" + id[:31], false},
		{"/stream/" + strings.Repeat("g", 32), false},
		{"/pls/" + id, false},
	} {
		got, ok := parseChannelIDPath(tc.path, "/stream/")
		if ok != tc.ok {
			t.Errorf("%q: ok=%v, want %v", tc.path, ok, tc.ok)
		}
		if ok && hex.EncodeToString(got[:]) != id {
			t.Errorf("%q: id=%x", tc.path, got)
		}
	}
}

func TestParseTip(t *testing.T) {
	for _, tc := range []struct {
		tip string
		ok  bool
	}{
		{"", true},
		{"203.0.113.9:7144", true},
		{"example.com:7144", true},
		{"[2001:db8::1]:7144", true},
		{"203.0.113.9", false},
		{":7144", false},
		{"203.0.113.9:0", false},
		{"203.0.113.9:65536", false},
		{"203.0.113.9:abc", false},
		{"http://203.0.113.9:7144", false},
	} {
		got, ok := parseTip(tc.tip)
		if ok != tc.ok || (ok && got != tc.tip) {
			t.Errorf("%q: got %q ok=%v, want ok=%v", tc.tip, got, ok, tc.ok)
		}
	}
}
