package jsonrpc

import (
	"net"
	"testing"

	"github.com/titagaki/peercast-mi/internal/channel"
	"github.com/titagaki/peercast-mi/internal/pcputil"
	"github.com/titagaki/peercast-pcp/pcp"
)

type reconnectManager struct {
	ChannelManager
	called pcp.GnuID
}

func (m *reconnectManager) Reconnect(id pcp.GnuID) error { m.called = id; return nil }

func TestCompatBumpRelay(t *testing.T) {
	s, _, _ := newTestServer(t)
	m := &reconnectManager{ChannelManager: s.mgr}
	s.mgr = m
	ch := channel.New(pcp.GnuID{1}, pcp.GnuID{}, 0)
	if _, err := s.bumpChannel(ch); err != nil {
		t.Fatal(err)
	}
	if m.called != ch.ID {
		t.Fatal("relay not reconnected")
	}
}

func TestCompatRelayTreeDescendantsAndCycles(t *testing.T) {
	s, ch, _ := newTestServer(t)
	parent, child := pcp.GnuID{1}, pcp.GnuID{2}
	host := func(id pcp.GnuID, ip, up string) *pcp.Atom {
		return pcputil.BuildHostAtom(pcputil.HostAtomParams{SessionID: id, GlobalAddress: net.ParseIP(ip), ListenPort: 7144, UpstreamAddress: net.ParseIP(up), UphostPort: 7144, NumListeners: 3, RelayFull: true, IsReceiving: true})
	}
	ch.ObserveHost(host(parent, "203.0.113.1", "203.0.113.99"), parent)
	ch.ObserveHost(host(child, "203.0.113.2", "203.0.113.1"), parent)
	v, err := s.getChannelRelayTree(ch)
	if err != nil {
		t.Fatal(err)
	}
	tree := v.([]relayTreeNode)
	if len(tree[0].Children) != 1 || len(tree[0].Children[0].Children) != 1 {
		t.Fatalf("missing descendant: %+v", tree)
	}
	n := tree[0].Children[0].Children[0]
	if n.SessionID != gnuIDString(child) || !n.IsReceiving || !n.IsRelayFull || n.LocalDirects != 3 {
		t.Fatalf("HOST fields not reflected: %+v", n)
	}
	ch.ObserveHost(host(parent, "203.0.113.1", "203.0.113.2"), parent)
	v, err = s.getChannelRelayTree(ch)
	if err != nil {
		t.Fatal(err)
	}
	var count func(relayTreeNode) int
	count = func(n relayTreeNode) int {
		total := 1
		for _, c := range n.Children {
			total += count(c)
		}
		return total
	}
	if count(v.([]relayTreeNode)[0]) != 3 {
		t.Fatal("cycle lost or duplicated nodes")
	}
}
