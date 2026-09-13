package servent

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/titagaki/peercast-pcp/pcp"

	"github.com/titagaki/peercast-mi/internal/channel"
)

const (
	testChannelHex = "000102030405060708090a0b0c0d0e0f"
	testTip        = "203.0.113.9:7144"
)

var (
	testChannelID = pcp.GnuID{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	flvHeader     = []byte("FLV\x01\x05\x00\x00\x00\x09\x00\x00\x00\x00")
	flvKeyframe   = []byte("\x09\x00\x00\x05keyframe")
)

// stubRelay は Manager.StartRelay から起動される relay client の代わり。Run で feed を
// 実行してチャンネルにデータを流し込み、Stop されるまで待つ。
type stubRelay struct {
	ch        *channel.Channel
	feed      func(ch *channel.Channel)
	stopCh    chan struct{}
	stopOnce  sync.Once
	onStopped func()
}

func newStubRelay(ch *channel.Channel, feed func(*channel.Channel)) *stubRelay {
	return &stubRelay{ch: ch, feed: feed, stopCh: make(chan struct{})}
}

func (r *stubRelay) Run() {
	if r.feed != nil {
		r.feed(r.ch)
	}
	<-r.stopCh
	if r.onStopped != nil {
		r.onStopped()
	}
}
func (r *stubRelay) Stop()                  { r.stopOnce.Do(func() { close(r.stopCh) }) }
func (r *stubRelay) SetGlobalIP(uint32)     {}
func (r *stubRelay) SetOnStopped(fn func()) { r.onStopped = fn }
func feedFLV(ch *channel.Channel)           { ch.SetHeader(flvHeader, 0); ch.Write(flvKeyframe, 1, 0) }
func feedNothing(*channel.Channel)          {}
func stubFactory(feed func(*channel.Channel)) channel.RelayFactory {
	return func(ch *channel.Channel, _ string) channel.RelayHandle { return newStubRelay(ch, feed) }
}

// newRelayListener は StartRelay で stubRelay を起動する Manager と、それを
// /pls/ と /stream/ の OnDemandRelay に結線した Listener を返す (main.go と同じ形)。
func newRelayListener(t *testing.T, factory channel.RelayFactory, maxListeners int) (*Listener, *channel.Manager) {
	t.Helper()
	mgr := channel.NewManager(pcp.GnuID{})
	mgr.NewRelay = factory
	t.Cleanup(mgr.StopAll)
	l := NewListener(pcp.GnuID{}, mgr, 7144, 0, 0, maxListeners, 0)
	l.RelayRequestFromAny = true // net.Pipe の remote はプライベート判定できない
	l.OnDemandRelay = func(id pcp.GnuID, tip string) (*channel.Channel, error) {
		return mgr.StartRelay(id, tip)
	}
	return l, mgr
}

// addrConn は RemoteAddr を差し替えた net.Conn (送信元判定のテスト用)。
type addrConn struct {
	net.Conn
	remote net.Addr
}

func (c addrConn) RemoteAddr() net.Addr { return c.remote }

// openStreamFrom は remote から来た接続として /stream/ を要求する。
func openStreamFrom(t *testing.T, l *Listener, remote net.Addr, path string) *streamViewer {
	t.Helper()
	server, client := net.Pipe()
	v := &streamViewer{client: client, done: make(chan struct{})}
	go func() {
		l.handleHTTPStream(newCountingConn(addrConn{server, remote}), bufio.NewReader(server))
		close(v.done)
	}()
	v.readHeaders(t, path)
	return v
}

// streamViewer は net.Pipe の client 側から /stream/ を要求する疑似プレイヤー。
type streamViewer struct {
	client net.Conn
	done   chan struct{} // handler が戻ったら閉じる
	resp   *http.Response
	br     *bufio.Reader
}

// openStream は handler を goroutine で起動し、リクエストを送ってレスポンスヘッダーまで読む。
func openStream(t *testing.T, l *Listener, path string) *streamViewer {
	t.Helper()
	server, client := net.Pipe()
	v := &streamViewer{client: client, done: make(chan struct{})}
	go func() {
		l.handleHTTPStream(newCountingConn(server), bufio.NewReader(server))
		close(v.done)
	}()
	v.readHeaders(t, path)
	return v
}

// readHeaders はリクエストを送ってレスポンスヘッダーまで読む。
func (v *streamViewer) readHeaders(t *testing.T, path string) {
	t.Helper()
	client := v.client
	type hdr struct {
		resp *http.Response
		err  error
	}
	hdrCh := make(chan hdr, 1)
	go func() {
		if _, err := io.WriteString(client, "GET "+path+" HTTP/1.0\r\nHost: localhost\r\n\r\n"); err != nil {
			hdrCh <- hdr{err: err}
			return
		}
		v.br = bufio.NewReader(client)
		resp, err := http.ReadResponse(v.br, nil)
		hdrCh <- hdr{resp: resp, err: err}
	}()
	select {
	case h := <-hdrCh:
		if h.err != nil {
			t.Fatalf("%s: read response: %v", path, h.err)
		}
		v.resp = h.resp
	case <-time.After(5 * time.Second):
		t.Fatalf("%s: no response headers", path)
	}
}

// readBody は body を n バイト (n < 0 なら EOF まで) 読む。
func (v *streamViewer) readBody(t *testing.T, n int) []byte {
	t.Helper()
	type res struct {
		b   []byte
		err error
	}
	ch := make(chan res, 1)
	go func() {
		if n < 0 {
			b, err := io.ReadAll(v.br)
			ch <- res{b, err}
			return
		}
		b := make([]byte, n)
		_, err := io.ReadFull(v.br, b)
		ch <- res{b, err}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("read body: %v", r.err)
		}
		return r.b
	case <-time.After(5 * time.Second):
		t.Fatal("body did not arrive")
		return nil
	}
}

// close は視聴者の切断をシミュレートし、handler が戻るのを待つ。
func (v *streamViewer) close(t *testing.T) {
	t.Helper()
	v.client.Close()
	select {
	case <-v.done:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not return after viewer disconnect")
	}
}

func wantFLV(t *testing.T, v *streamViewer) {
	t.Helper()
	if v.resp.StatusCode != 200 {
		t.Fatalf("status %d, want 200", v.resp.StatusCode)
	}
	want := append(append([]byte(nil), flvHeader...), flvKeyframe...)
	if got := v.readBody(t, len(want)); !bytes.Equal(got, want) {
		t.Fatalf("body = %q, want header+keyframe %q", got, want)
	}
}

// TestHTTPStream_RegisteredChannel は登録済みチャンネルの視聴が同じ接続で
// ヘッダーとデータを返し、tip が付いていても OnDemandRelay を呼ばないことを確認する。
func TestHTTPStream_RegisteredChannel(t *testing.T) {
	l, mgr := newRelayListener(t, stubFactory(feedFLV), 0)
	ch := channel.New(testChannelID, pcp.GnuID{}, 0)
	feedFLV(ch)
	mgr.AddRelayChannel(ch, newStubRelay(ch, nil))
	var relayCalls atomic.Int32
	l.OnDemandRelay = func(pcp.GnuID, string) (*channel.Channel, error) {
		relayCalls.Add(1)
		return nil, errors.New("must not be called")
	}

	for _, path := range []string{
		"/stream/" + testChannelHex,
		"/stream/" + testChannelHex + ".flv",
		"/stream/" + testChannelHex + ".flv?tip=" + testTip,
	} {
		v := openStream(t, l, path)
		wantFLV(t, v)
		if ch.NumListeners() != 1 {
			t.Fatalf("%s: listeners = %d, want 1", path, ch.NumListeners())
		}
		v.close(t)
		if ch.NumListeners() != 0 {
			t.Fatalf("%s: listeners after disconnect = %d, want 0", path, ch.NumListeners())
		}
	}
	if relayCalls.Load() != 0 {
		t.Fatalf("OnDemandRelay called %d times for a registered channel", relayCalls.Load())
	}
	if _, ok := mgr.GetByID(testChannelID); !ok {
		t.Fatal("viewer disconnect must not remove the shared channel")
	}
}

// TestHTTPStream_AutoRelay は未登録チャンネルへの /stream/?tip= がリレーを開始し、
// リレーが受け取った FLV ヘッダーとデータを同じ HTTP 接続で返すことを確認する。
func TestHTTPStream_AutoRelay(t *testing.T) {
	for _, ext := range []string{"", ".flv"} {
		var gotAddr string
		l, mgr := newRelayListener(t, func(ch *channel.Channel, addr string) channel.RelayHandle {
			gotAddr = addr
			// 上流からのデータはハンドシェイク後に届くので少し遅らせる。
			return newStubRelay(ch, func(ch *channel.Channel) {
				time.Sleep(20 * time.Millisecond)
				feedFLV(ch)
			})
		}, 0)

		v := openStream(t, l, "/stream/"+testChannelHex+ext+"?tip="+testTip)
		wantFLV(t, v)
		if gotAddr != testTip {
			t.Fatalf("ext %q: relay started with %q, want %q", ext, gotAddr, testTip)
		}
		ch, ok := mgr.GetByID(testChannelID)
		if !ok || ch.UpstreamAddr() != testTip {
			t.Fatalf("ext %q: channel not registered with upstream %q", ext, testTip)
		}
		v.close(t)
		if _, ok := mgr.GetByID(testChannelID); !ok {
			t.Fatalf("ext %q: relay removed when the viewer disconnected", ext)
		}
		mgr.StopAll()
	}
}

// TestHTTPStream_ConcurrentAutoRelay は同じ未登録チャンネルへの同時アクセスでも
// リレーが 1 つしか生成されないことを確認する。
func TestHTTPStream_ConcurrentAutoRelay(t *testing.T) {
	var factoryCalls atomic.Int32
	l, mgr := newRelayListener(t, func(ch *channel.Channel, _ string) channel.RelayHandle {
		factoryCalls.Add(1)
		return newStubRelay(ch, func(ch *channel.Channel) {
			time.Sleep(20 * time.Millisecond)
			feedFLV(ch)
		})
	}, 0)

	// 並列サブテストで同時に接続する (goroutine から t.Fatal を呼ばないため)。
	t.Run("viewers", func(t *testing.T) {
		for i := 0; i < 8; i++ {
			t.Run(strconv.Itoa(i), func(t *testing.T) {
				t.Parallel()
				v := openStream(t, l, "/stream/"+testChannelHex+".flv?tip="+testTip)
				wantFLV(t, v)
				v.close(t)
			})
		}
	})
	if factoryCalls.Load() != 1 {
		t.Fatalf("relay factory called %d times, want 1", factoryCalls.Load())
	}
	ch, ok := mgr.GetByID(testChannelID)
	if !ok {
		t.Fatal("channel not registered")
	}
	if ch.NumListeners() != 0 {
		t.Fatalf("listeners after all viewers left = %d, want 0", ch.NumListeners())
	}
}

// TestHTTPStream_Errors はレスポンス送信前に検出できる異常が HTTP エラーになることを確認する。
func TestHTTPStream_Errors(t *testing.T) {
	t.Run("bad channel id", func(t *testing.T) {
		l, _ := newRelayListener(t, stubFactory(feedFLV), 0)
		v := openStream(t, l, "/stream/not-a-channel.flv?tip="+testTip)
		if v.resp.StatusCode != 400 {
			t.Fatalf("status %d, want 400", v.resp.StatusCode)
		}
		v.close(t)
	})
	t.Run("bad tip", func(t *testing.T) {
		l, _ := newRelayListener(t, stubFactory(feedFLV), 0)
		for _, tip := range []string{"203.0.113.9", "203.0.113.9:99999", "pcp://203.0.113.9:7144"} {
			v := openStream(t, l, "/stream/"+testChannelHex+".flv?tip="+tip)
			if v.resp.StatusCode != 400 {
				t.Fatalf("tip %q: status %d, want 400", tip, v.resp.StatusCode)
			}
			v.close(t)
		}
	})
	t.Run("unregistered without on-demand relay", func(t *testing.T) {
		l := NewListener(pcp.GnuID{}, notFoundStore{}, 7144, 0, 0, 0, 0)
		v := openStream(t, l, "/stream/"+testChannelHex+".flv?tip="+testTip)
		if v.resp.StatusCode != 404 {
			t.Fatalf("status %d, want 404", v.resp.StatusCode)
		}
		v.close(t)
	})
	t.Run("relay start failure", func(t *testing.T) {
		l, _ := newRelayListener(t, stubFactory(feedFLV), 0)
		called := false
		l.OnDemandRelay = func(id pcp.GnuID, tip string) (*channel.Channel, error) {
			called = true
			return nil, errors.New("tracker not found")
		}
		v := openStream(t, l, "/stream/"+testChannelHex+".flv")
		if v.resp.StatusCode != 404 {
			t.Fatalf("status %d, want 404", v.resp.StatusCode)
		}
		if !called {
			t.Fatal("OnDemandRelay not called for an unregistered channel without tip")
		}
		v.close(t)
	})
	t.Run("listener limit", func(t *testing.T) {
		l, mgr := newRelayListener(t, stubFactory(feedFLV), 1)
		ch := channel.New(testChannelID, pcp.GnuID{}, 0)
		feedFLV(ch)
		mgr.AddRelayChannel(ch, newStubRelay(ch, nil))
		first := openStream(t, l, "/stream/"+testChannelHex+".flv")
		wantFLV(t, first)
		second := openStream(t, l, "/stream/"+testChannelHex+".flv")
		if second.resp.StatusCode != 503 {
			t.Fatalf("status %d, want 503", second.resp.StatusCode)
		}
		second.close(t)
		first.close(t)
	})
}

// TestHTTPStream_FirstDataTimeout はリレーからデータが届かないとき、200 送信後に
// 待機上限で接続を閉じ (別のエラーは書かず)、リレーは残ることを確認する。
func TestHTTPStream_FirstDataTimeout(t *testing.T) {
	old := firstDataTimeout
	firstDataTimeout = 50 * time.Millisecond
	t.Cleanup(func() { firstDataTimeout = old })

	l, mgr := newRelayListener(t, stubFactory(feedNothing), 0)
	v := openStream(t, l, "/stream/"+testChannelHex+".flv?tip="+testTip)
	if v.resp.StatusCode != 200 {
		t.Fatalf("status %d, want 200", v.resp.StatusCode)
	}
	if body := v.readBody(t, -1); len(body) != 0 {
		t.Fatalf("body = %q, want empty", body)
	}
	select {
	case <-v.done:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not return after first-data timeout")
	}
	ch, ok := mgr.GetByID(testChannelID)
	if !ok {
		t.Fatal("relay removed by a viewer timeout")
	}
	if ch.NumListeners() != 0 {
		t.Fatalf("listeners = %d, want 0", ch.NumListeners())
	}
}

// TestHTTPStream_RelayStopped はリレー終了 (Manager.Stop → CloseAll) で視聴者の接続が
// 閉じられ、待機処理が残らないことを確認する。
func TestHTTPStream_RelayStopped(t *testing.T) {
	l, mgr := newRelayListener(t, stubFactory(feedFLV), 0)
	v := openStream(t, l, "/stream/"+testChannelHex+".flv?tip="+testTip)
	wantFLV(t, v)

	mgr.Stop(testChannelID)
	if body := v.readBody(t, -1); len(body) != 0 {
		t.Fatalf("unexpected trailing body %q", body)
	}
	select {
	case <-v.done:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not return after the relay stopped")
	}
}

// TestHTTPStream_RelayRequestFrom は既定 (private) では未登録チャンネルのリレー開始を
// ループバック・プライベートアドレスからしか受け付けず、登録済みチャンネルの視聴は
// 送信元によらず許可することを確認する。
func TestHTTPStream_RelayRequestFrom(t *testing.T) {
	global := &net.TCPAddr{IP: net.ParseIP("203.0.113.9"), Port: 50000}
	private := []net.Addr{
		&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 50000},
		&net.TCPAddr{IP: net.ParseIP("::1"), Port: 50000},
		&net.TCPAddr{IP: net.ParseIP("192.168.1.10"), Port: 50000},
		&net.TCPAddr{IP: net.ParseIP("10.0.0.5"), Port: 50000},
		&net.TCPAddr{IP: net.ParseIP("172.16.0.5"), Port: 50000},
		&net.TCPAddr{IP: net.ParseIP("fd00::5"), Port: 50000},
	}
	path := "/stream/" + testChannelHex + ".flv?tip=" + testTip

	t.Run("global remote refused", func(t *testing.T) {
		l, mgr := newRelayListener(t, stubFactory(feedFLV), 0)
		l.RelayRequestFromAny = false
		v := openStreamFrom(t, l, global, path)
		if v.resp.StatusCode != 403 {
			t.Fatalf("status %d, want 403", v.resp.StatusCode)
		}
		v.close(t)
		if _, ok := mgr.GetByID(testChannelID); ok {
			t.Fatal("relay must not be started for a refused remote")
		}
	})
	t.Run("private remotes allowed", func(t *testing.T) {
		for _, remote := range private {
			l, mgr := newRelayListener(t, stubFactory(feedFLV), 0)
			l.RelayRequestFromAny = false
			v := openStreamFrom(t, l, remote, path)
			wantFLV(t, v)
			v.close(t)
			mgr.StopAll()
		}
	})
	t.Run("global remote may view a registered channel", func(t *testing.T) {
		l, mgr := newRelayListener(t, stubFactory(feedFLV), 0)
		l.RelayRequestFromAny = false
		ch := channel.New(testChannelID, pcp.GnuID{}, 0)
		feedFLV(ch)
		mgr.AddRelayChannel(ch, newStubRelay(ch, nil))
		v := openStreamFrom(t, l, global, path)
		wantFLV(t, v)
		v.close(t)
	})
	t.Run("any", func(t *testing.T) {
		l, _ := newRelayListener(t, stubFactory(feedFLV), 0)
		l.RelayRequestFromAny = true
		v := openStreamFrom(t, l, global, path)
		wantFLV(t, v)
		v.close(t)
	})
}

// TestHTTPStream_RelayChannelLimit は max_relay_channels に達したら新しいリレーを
// 503 で断り、既存のリレーチャンネルの視聴は続けられることを確認する。
func TestHTTPStream_RelayChannelLimit(t *testing.T) {
	l, mgr := newRelayListener(t, stubFactory(feedFLV), 0)
	mgr.MaxRelayChannels = 1

	first := openStream(t, l, "/stream/"+testChannelHex+".flv?tip="+testTip)
	wantFLV(t, first)

	const otherHex = "101112131415161718191a1b1c1d1e1f"
	second := openStream(t, l, "/stream/"+otherHex+".flv?tip="+testTip)
	if second.resp.StatusCode != 503 {
		t.Fatalf("status %d, want 503", second.resp.StatusCode)
	}
	second.close(t)

	again := openStream(t, l, "/stream/"+testChannelHex+".flv")
	wantFLV(t, again)
	again.close(t)
	first.close(t)
}
