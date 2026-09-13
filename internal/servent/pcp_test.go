package servent

import (
	"bufio"
	"net"
	"testing"
	"time"

	"github.com/titagaki/peercast-pcp/pcp"

	"github.com/titagaki/peercast-mi/internal/channel"
)

// newTestOutputStream は net.Pipe を使って PCPOutputStream とピア接続を返す。
// テスト終了時に peer.Close() と out.Close() を呼ぶこと。
func newTestOutputStream(t *testing.T) (*PCPOutputStream, net.Conn) {
	t.Helper()
	c1, c2 := net.Pipe()
	cc := newCountingConn(c1)
	br := bufio.NewReader(cc)
	var sid pcp.GnuID
	out := newPCPOutputStream(cc, br, sid, channel.New(pcp.GnuID{}, pcp.GnuID{}, 0), 42, 0, 0, 0, 0)
	return out, c2
}

// --- notify ---

// TestNotify_NonBlocking は最初の通知がチャンネルに届き、2 回目がブロックしないことを確認する。
func TestNotify_NonBlocking(t *testing.T) {
	ch := make(chan struct{}, 1)
	notify(ch)
	notify(ch) // バッファ満杯でもブロックしない
	if len(ch) != 1 {
		t.Errorf("channel length: got %d, want 1", len(ch))
	}
}

// --- ipToUint32 ---

// TestIPToUint32 は IPv4 アドレスを uint32 へ正しく変換することを確認する。
func TestIPToUint32(t *testing.T) {
	tests := []struct {
		name string
		addr net.Addr
		want uint32
	}{
		{
			name: "1.2.3.4",
			addr: &net.TCPAddr{IP: net.IP{1, 2, 3, 4}},
			want: 0x01020304,
		},
		{
			name: "192.168.1.100",
			addr: &net.TCPAddr{IP: net.IPv4(192, 168, 1, 100)},
			want: 0xC0A80164,
		},
		{
			name: "nil IP",
			addr: &net.TCPAddr{IP: nil},
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ipToUint32(tt.addr)
			if got != tt.want {
				t.Errorf("ipToUint32: got 0x%08X, want 0x%08X", got, tt.want)
			}
		})
	}
}

// TestIPToUint32_NonTCPAddr は *net.TCPAddr 以外で 0 を返すことを確認する。
func TestIPToUint32_NonTCPAddr(t *testing.T) {
	addr := &net.UDPAddr{IP: net.IP{1, 2, 3, 4}}
	if got := ipToUint32(addr); got != 0 {
		t.Errorf("ipToUint32(UDP): got %d, want 0", got)
	}
}

// --- Atom builders ---

// TestBuildChanInfoAtom は PCPChanInfo タグを持つアトムが生成されることを確認する。
func TestBuildChanInfoAtom(t *testing.T) {
	info := channel.ChannelInfo{Name: "test", Genre: "music", Bitrate: 128}
	ci := info.ToPCP()
	a := ci.BuildAtom()
	if a.Tag != pcp.PCPChanInfo {
		t.Errorf("Tag: got %v, want PCPChanInfo", a.Tag)
	}
}

// TestBuildChanTrackAtom は PCPChanTrack タグを持つアトムが生成されることを確認する。
func TestBuildChanTrackAtom(t *testing.T) {
	track := channel.TrackInfo{Title: "My Track", Creator: "Artist"}
	ct := track.ToPCP()
	a := ct.BuildAtom()
	if a.Tag != pcp.PCPChanTrack {
		t.Errorf("Tag: got %v, want PCPChanTrack", a.Tag)
	}
}

// TestBuildChanAtom はトップレベルが PCPChan タグを持つアトムが生成されることを確認する。
func TestBuildChanAtom(t *testing.T) {
	var id pcp.GnuID
	a := buildChanAtom(id, id, channel.ChannelInfo{Name: "test"}, channel.TrackInfo{}, []byte{0x01}, 0)
	if a.Tag != pcp.PCPChan {
		t.Errorf("Tag: got %v, want PCPChan", a.Tag)
	}
}

// --- PCPOutputStream ---

// TestPCPOutputStream_Type は Type() が OutputStreamPCP を返すことを確認する。
func TestPCPOutputStream_Type(t *testing.T) {
	out, peer := newTestOutputStream(t)
	defer peer.Close()
	defer out.Close()

	if got := out.Type(); got != channel.OutputStreamPCP {
		t.Errorf("Type: got %v, want OutputStreamPCP", got)
	}
}

// TestPCPOutputStream_ID は ID() がコンストラクタで指定した値を返すことを確認する。
func TestPCPOutputStream_ID(t *testing.T) {
	out, peer := newTestOutputStream(t)
	defer peer.Close()
	defer out.Close()

	if got := out.ID(); got != 42 {
		t.Errorf("ID: got %d, want 42", got)
	}
}

// TestPCPOutputStream_RemoteAddr は RemoteAddr() が空でない文字列を返すことを確認する。
func TestPCPOutputStream_RemoteAddr(t *testing.T) {
	out, peer := newTestOutputStream(t)
	defer peer.Close()
	defer out.Close()

	if got := out.RemoteAddr(); got == "" {
		t.Error("RemoteAddr: got empty string")
	}
}

// TestPCPOutputStream_SendRate_Initial は初期 SendRate が 0 であることを確認する。
func TestPCPOutputStream_SendRate_Initial(t *testing.T) {
	out, peer := newTestOutputStream(t)
	defer peer.Close()
	defer out.Close()

	if got := out.SendRate(); got != 0 {
		t.Errorf("SendRate (initial): got %d, want 0", got)
	}
}

// TestPCPOutputStream_NotifyHeader は headerCh にシグナルが送られることを確認する。
func TestPCPOutputStream_NotifyHeader(t *testing.T) {
	out, peer := newTestOutputStream(t)
	defer peer.Close()
	defer out.Close()

	out.NotifyHeader()
	if len(out.headerCh) != 1 {
		t.Error("NotifyHeader: headerCh not signaled")
	}
}

// TestPCPOutputStream_NotifyInfo は infoCh にシグナルが送られることを確認する。
func TestPCPOutputStream_NotifyInfo(t *testing.T) {
	out, peer := newTestOutputStream(t)
	defer peer.Close()
	defer out.Close()

	out.NotifyInfo()
	if len(out.infoCh) != 1 {
		t.Error("NotifyInfo: infoCh not signaled")
	}
}

// TestPCPOutputStream_NotifyTrack は trackCh にシグナルが送られることを確認する。
func TestPCPOutputStream_NotifyTrack(t *testing.T) {
	out, peer := newTestOutputStream(t)
	defer peer.Close()
	defer out.Close()

	out.NotifyTrack()
	if len(out.trackCh) != 1 {
		t.Error("NotifyTrack: trackCh not signaled")
	}
}

// TestPCPOutputStream_Close_Idempotent は Close が冪等でパニックしないことを確認する。
func TestPCPOutputStream_Close_Idempotent(t *testing.T) {
	out, peer := newTestOutputStream(t)
	defer peer.Close()

	out.Close()
	out.Close() // 2 回目はパニックしない

	out.mu.Lock()
	closed := out.closed
	out.mu.Unlock()
	if !closed {
		t.Error("closed: expected true after Close")
	}
}

// TestPCPOutputStream_Close_SignalsCloseCh は Close が closeCh を閉じることを確認する。
func TestPCPOutputStream_Close_SignalsCloseCh(t *testing.T) {
	out, peer := newTestOutputStream(t)
	defer peer.Close()

	out.Close()

	select {
	case <-out.closeCh:
		// OK
	default:
		t.Error("closeCh: expected closed after Close")
	}
}

// TestPCPOutputStream_Evict_SendsAlternativesAndUnavailable は MakeRelayable による
// 退出時に、上流ノードではなく代替候補 (要求元自身を除く) と QUIT+UNAVAILABLE を
// 送ることを確認する。
func TestPCPOutputStream_Evict_SendsAlternativesAndUnavailable(t *testing.T) {
	out, peer := newTestOutputStream(t)
	defer peer.Close()
	out.peerID = pcp.GnuID{1}
	out.ch.SetUpstreamNodeInfo(pcp.GnuID{9}, 0x0a000009, 7144)
	out.ch.AddKnownHost(pcp.NewParentAtom(pcp.PCPHost, pcp.NewIDAtom(pcp.PCPHostID, pcp.GnuID{1}))) // 要求元自身
	out.ch.AddKnownHost(pcp.NewParentAtom(pcp.PCPHost, pcp.NewIDAtom(pcp.PCPHostID, pcp.GnuID{2})))

	done := make(chan struct{})
	go func() {
		defer close(done)
		out.streamLoop(0)
	}()
	out.Evict()

	host, err := pcp.ReadAtom(peer)
	if err != nil {
		t.Fatalf("read host: %v", err)
	}
	if host.Tag != pcp.PCPHost {
		t.Fatalf("first atom = %v, want host", host.Tag)
	}
	if sid, _ := host.FindChild(pcp.PCPHostID).GetID(); sid != (pcp.GnuID{2}) {
		t.Fatalf("host sid = %v, want the other node (not requester, not upstream)", sid)
	}
	quit, err := pcp.ReadAtom(peer)
	if err != nil {
		t.Fatalf("read quit: %v", err)
	}
	if quit.Tag != pcp.PCPQuit {
		t.Fatalf("second atom = %v, want quit", quit.Tag)
	}
	if code, _ := quit.GetInt(); code != pcp.PCPErrorQuit+pcp.PCPErrorUnavailable {
		t.Fatalf("quit code = %d, want QUIT+UNAVAILABLE", code)
	}
	<-done
}

// TestCanAdmitRelay_Banned は BAN 中の IP からのリレー要求を枠の有無にかかわらず拒否することを確認する。
func TestCanAdmitRelay_Banned(t *testing.T) {
	l := &Listener{mgr: channel.NewManager(pcp.GnuID{}), maxRelays: 10}
	ch := channel.New(pcp.GnuID{1}, pcp.GnuID{}, 0)
	remote := &net.TCPAddr{IP: net.IPv4(10, 0, 0, 2), Port: 51000}
	if !l.canAdmitRelay(ch, remote) {
		t.Fatal("unbanned remote with free slots must be admitted")
	}
	ch.Ban("10.0.0.2", time.Now().Add(time.Minute))
	if l.canAdmitRelay(ch, remote) {
		t.Fatal("banned remote must be refused")
	}
	if !l.canAdmitRelay(ch, &net.TCPAddr{IP: net.IPv4(10, 0, 0, 3), Port: 51000}) {
		t.Fatal("other remote must still be admitted")
	}
}

// --- streamLoop の開始位置と位置の巻き戻り ---

// readPktPositions は peer から n 個の chan/pkt アトムを読み、(type, pos) を返す。
func readPktPositions(t *testing.T, peer net.Conn, n int) (types []string, positions []uint32) {
	t.Helper()
	for i := 0; i < n; i++ {
		peer.SetReadDeadline(time.Now().Add(2 * time.Second))
		atom, err := pcp.ReadAtom(peer)
		if err != nil {
			t.Fatalf("read atom %d: %v", i, err)
		}
		if atom.Tag != pcp.PCPChan {
			t.Fatalf("atom %d = %v, want chan", i, atom.Tag)
		}
		cp, err := pcp.ParseChanPacket(atom)
		if err != nil || cp.Pkt == nil {
			t.Fatalf("atom %d: not a pkt: %v", i, err)
		}
		types = append(types, cp.Pkt.Type.String())
		positions = append(positions, cp.Pkt.Pos)
	}
	return types, positions
}

// runStreamLoop は streamLoop を goroutine で起動し、停止用の関数を返す。
func runStreamLoop(t *testing.T, out *PCPOutputStream, peer net.Conn, reqPos uint32) func() {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		out.streamLoop(reqPos)
	}()
	return func() {
		out.Close()
		peer.Close()
		<-done
	}
}

// TestStreamLoop_ResumeAcrossWrap は位置が 2^32 で一周した直後に x-peercast-pos で
// 再開しても、一周前のパケットを送り直さず要求位置から送ることを確認する。
func TestStreamLoop_ResumeAcrossWrap(t *testing.T) {
	out, peer := newTestOutputStream(t)
	out.ch.SetHeader([]byte("hdr"), 0xFFFFFC00)
	out.ch.Write(make([]byte, 0x200), 0xFFFFFC00, 0)
	out.ch.Write(make([]byte, 0x200), 0xFFFFFE00, 0)
	out.ch.Write(make([]byte, 0x200), 0x00000000, 0) // 一周
	out.ch.Write(make([]byte, 0x200), 0x00000200, 0)
	out.ch.Write(make([]byte, 0x200), 0x00000400, 0)

	stop := runStreamLoop(t, out, peer, 0x00000200)
	defer stop()
	_, pos := readPktPositions(t, peer, 2)
	if pos[0] != 0x200 || pos[1] != 0x400 {
		t.Fatalf("positions = %#x, want [0x200 0x400]", pos)
	}
}

// TestStreamLoop_ReqPosBehindBufferStartsFromOldest はバッファから溢れた位置を
// 要求されたとき最古のパケットから送ることを確認する (一周をまたぐ場合も)。
func TestStreamLoop_ReqPosBehindBufferStartsFromOldest(t *testing.T) {
	out, peer := newTestOutputStream(t)
	out.ch.SetHeader([]byte("hdr"), 0xFFFFF000)
	out.ch.Write(make([]byte, 0x200), 0xFFFFFE00, 0)
	out.ch.Write(make([]byte, 0x200), 0x00000000, 0)

	stop := runStreamLoop(t, out, peer, 0xFFFFF000)
	defer stop()
	_, pos := readPktPositions(t, peer, 2)
	if pos[0] != 0xFFFFFE00 || pos[1] != 0 {
		t.Fatalf("positions = %#x, want [0xFFFFFE00 0]", pos)
	}
}

// TestStartCursor は x-peercast-pos の要求位置をバッファの範囲に丸める規則を確認する。
func TestStartCursor(t *testing.T) {
	out, peer := newTestOutputStream(t)
	defer peer.Close()

	// バッファが空: 要求位置によらずヘッダー位置。
	out.ch.SetHeader([]byte("hdr"), 0x1000)
	for _, reqPos := range []uint32{0, 0x800, 0x2000} {
		if cur := out.startCursor(reqPos); cur.pos != 0x1000 || !cur.waitingForKeyframe {
			t.Fatalf("empty buffer, reqPos %#x: cursor = %+v, want pos 0x1000 waiting for keyframe", reqPos, cur)
		}
	}

	out.ch.Write(make([]byte, 0x100), 0x1003, 0)
	out.ch.Write(make([]byte, 0x100), 0x1103, 0)
	for _, tc := range []struct {
		name    string
		reqPos  uint32
		wantPos uint32
	}{
		{"unspecified → oldest", 0, 0x1003},
		{"behind oldest → oldest", 0x800, 0x1003},
		{"inside buffer → as requested", 0x1103, 0x1103},
		{"newest end → as requested", 0x1203, 0x1203},
		{"ahead of buffer → newest end (skip backlog)", 0x10000000, 0x1203},
		{"more than 2^31 ahead reads as behind → oldest", 0x90000000, 0x1003},
	} {
		if cur := out.startCursor(tc.reqPos); cur.pos != tc.wantPos {
			t.Errorf("%s: reqPos %#x → pos %#x, want %#x", tc.name, tc.reqPos, cur.pos, tc.wantPos)
		}
	}
}

// TestStreamLoop_UnspecifiedStartsFromOldest は x-peercast-pos なしのとき、ヘッダー位置が
// データから 2^31 以上離れていても (同じヘッダーを再送し続ける上流) バッファ全体を送ることを確認する。
func TestStreamLoop_UnspecifiedStartsFromOldest(t *testing.T) {
	out, peer := newTestOutputStream(t)
	out.ch.SetHeader([]byte("hdr"), 0)
	out.ch.Write(make([]byte, 0x100), 0x90000000, 0)
	out.ch.Write(make([]byte, 0x100), 0x90000100, 0)

	stop := runStreamLoop(t, out, peer, 0)
	defer stop()
	_, pos := readPktPositions(t, peer, 2)
	if pos[0] != 0x90000000 || pos[1] != 0x90000100 {
		t.Fatalf("positions = %#x, want [0x90000000 0x90000100]", pos)
	}
}

// TestStreamLoop_HeaderChangeRestartsFromNewHeader はヘッダー変更で位置が巻き戻っても
// (エンコーダー再接続など)、新ヘッダーを送ってから新しい位置のデータを続けることを確認する。
func TestStreamLoop_HeaderChangeRestartsFromNewHeader(t *testing.T) {
	out, peer := newTestOutputStream(t)
	out.ch.SetHeader([]byte("hdr1"), 0x40000000)
	out.ch.TryAddOutput(out, 0, 0) // 以降の SetHeader の通知を受け取る
	out.ch.Write(make([]byte, 0x100), 0x40000004, 0)

	stop := runStreamLoop(t, out, peer, 0)
	defer stop()
	if _, pos := readPktPositions(t, peer, 1); pos[0] != 0x40000004 {
		t.Fatalf("first position = %#x, want 0x40000004", pos[0])
	}

	// 新しいストリーム: ヘッダーもデータも位置 0 付近から。
	out.ch.SetHeader([]byte("hdr2"), 0)
	out.ch.Write(make([]byte, 0x100), 4, 0x02) // 非キーフレームはヘッダー直後に送らない
	out.ch.Write(make([]byte, 0x100), 0x104, 0)

	types, pos := readPktPositions(t, peer, 2)
	if types[0] != "head" || pos[0] != 0 {
		t.Fatalf("after header change: got %s@%#x, want head@0", types[0], pos[0])
	}
	if types[1] != "data" || pos[1] != 0x104 {
		t.Fatalf("after header: got %s@%#x, want data@0x104 (keyframe)", types[1], pos[1])
	}
}
