package yp

import (
	"net"
	"testing"
	"time"

	"github.com/titagaki/peercast-mi/internal/channel"
	"github.com/titagaki/peercast-mi/internal/pcputil"
	"github.com/titagaki/peercast-pcp/pcp"
)

func TestCompatAnnouncementChangesAndRemoval(t *testing.T) {
	m := channel.NewManager(pcp.GnuID{1})
	m.IssueStreamKey("a", "a")
	ch, err := m.Broadcast("a", channel.ChannelInfo{Name: "first"}, channel.TrackInfo{})
	if err != nil {
		t.Fatal(err)
	}
	ch.Write([]byte("data"), 0, 0)
	c := New("unused", pcp.GnuID{2}, pcp.GnuID{1}, m, 7144, 0, 0)
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	conn := &pcp.Conn{Conn: a}
	read := func() *pcp.Atom {
		t.Helper()
		done := make(chan error, 1)
		go func() { done <- c.sendAnnouncements(conn, false) }()
		b.SetReadDeadline(time.Now().Add(time.Second))
		atom, err := pcp.ReadAtom(b)
		if err != nil {
			t.Fatal(err)
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		return atom
	}
	first := read()
	if pcputil.Byte(first.FindChild(pcp.PCPHost), pcp.PCPHostFlags1)&pcp.PCPHostFlags1Recv == 0 {
		t.Fatal("initial not receiving")
	}
	// Unchanged snapshots must not write (net.Pipe would block).
	if err := c.sendAnnouncements(conn, false); err != nil {
		t.Fatal(err)
	}
	ch.SetInfo(channel.ChannelInfo{Name: "changed"})
	after := read()
	name := after.FindChild(pcp.PCPChan).FindChild(pcp.PCPChanInfo).FindChild(pcp.PCPChanInfoName)
	if name.GetString() != "changed" {
		t.Fatal("metadata change missing")
	}
	m.Stop(ch.ID)
	last := read()
	if pcputil.Byte(last.FindChild(pcp.PCPHost), pcp.PCPHostFlags1)&pcp.PCPHostFlags1Recv != 0 {
		t.Fatal("removed channel still receiving")
	}
	if len(c.announced) != 0 {
		t.Fatal("removed announcement retained")
	}
}
