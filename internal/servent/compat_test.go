package servent

import (
	"net"
	"testing"
	"time"

	"github.com/titagaki/peercast-mi/internal/channel"
	"github.com/titagaki/peercast-pcp/pcp"
)

func TestCompatMinimalPingThroughListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	sid := pcp.GnuID{1}
	l := NewListener(sid, nil, 0, 0, 0, 0, 0)
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err == nil {
			l.handle(conn)
		}
	}()
	port := uint16(ln.Addr().(*net.TCPAddr).Port)
	if !pingHost(net.ParseIP("127.0.0.1"), port, sid, pcp.GnuID{2}) {
		t.Fatal("minimal ping blocked by protocol detection")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("ping handler did not exit")
	}
}

func TestCompatOldBacklogAndAudio(t *testing.T) {
	for _, audio := range []bool{false, true} {
		out, peer := newTestOutputStream(t)
		defer out.Close()
		defer peer.Close()
		out.ch.SetHeader([]byte{'F', 'L', 'V', 1, 4}, 0)
		flags := byte(0)
		if audio {
			flags = 4
		}
		cur := out.startCursor(0)
		done := make(chan error, 1)
		go func() {
			done <- out.sendDataPackets(&cur, []channel.Content{{Data: []byte("frame"), Timestamp: time.Now().Add(-6 * time.Second), ContFlags: flags}})
		}()
		readPktPositions(t, peer, 1)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if cur.waitingForKeyframe {
			t.Fatal("complete audio-only frame was skipped")
		}
	}
}

func TestCompatActualQueueOverflow(t *testing.T) {
	out, peer := newTestOutputStream(t)
	defer out.Close()
	defer peer.Close()
	cur := streamCursor{started: time.Now().Add(-7 * time.Second)}
	done := make(chan error, 1)
	go func() {
		done <- out.sendDataPackets(&cur, []channel.Content{{Data: []byte("late"), Timestamp: time.Now().Add(-6 * time.Second)}})
	}()
	peer.SetReadDeadline(time.Now().Add(time.Second))
	a, err := pcp.ReadAtom(peer)
	if err != nil {
		t.Fatal(err)
	}
	if a.Tag != pcp.PCPQuit {
		t.Fatal("expected overflow QUIT")
	}
	if err := <-done; err == nil {
		t.Fatal("expected overflow error")
	}
}

func TestCompatIdleDoesNotDisconnect(t *testing.T) {
	out, peer := newTestOutputStream(t)
	stop := runStreamLoop(t, out, peer, 0)
	defer stop()
	peer.SetReadDeadline(time.Now().Add(5200 * time.Millisecond))
	if a, err := pcp.ReadAtom(peer); err == nil {
		t.Fatalf("idle stream sent %v", a.Tag)
	}
	out.ch.Write([]byte("resumed"), 0, 0)
	readPktPositions(t, peer, 1)
}
