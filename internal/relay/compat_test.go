package relay

import (
	"net"
	"testing"
	"time"

	"github.com/titagaki/peercast-mi/internal/channel"
	"github.com/titagaki/peercast-mi/internal/pcputil"
	"github.com/titagaki/peercast-pcp/pcp"
)

func TestCompatHandshakeTimeoutAndBump(t *testing.T) {
	for _, bump := range []bool{false, true} {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()
		ch := channel.New(pcp.GnuID{1}, pcp.GnuID{}, 0)
		c := New(ln.Addr().String(), ch.ID, pcp.GnuID{2}, 7144, ch)
		defer c.cancel()
		c.HandshakeTimeout = 100 * time.Millisecond
		if bump {
			c.HandshakeTimeout = time.Minute
		}
		done := make(chan error, 1)
		go func() { _, err := c.connectTo(ln.Addr().String()); done <- err }()
		conn, err := ln.Accept()
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		if bump {
			c.Reconnect()
		}
		select {
		case err := <-done:
			if err == nil {
				t.Fatal("silent handshake succeeded")
			}
		case <-time.After(time.Second):
			t.Fatal("silent handshake did not stop")
		}
		if c.ctx.Err() != nil {
			t.Fatal("bump cancelled entire relay")
		}
	}
}

func TestCompatIPv6SourceNode(t *testing.T) {
	h := pcputil.BuildHostAtom(pcputil.HostAtomParams{SessionID: pcp.GnuID{1}, GlobalAddress: net.ParseIP("2001:db8::1"), LocalAddress: net.ParseIP("fd00::1"), ListenPort: 7144, IsReceiving: true})
	n, ok := parseSourceNode(h)
	if !ok || n.GlobalAddr != "[2001:db8::1]:7144" || n.LocalAddr != "[fd00::1]:7144" {
		t.Fatalf("IPv6 endpoints: %+v", n)
	}
}
