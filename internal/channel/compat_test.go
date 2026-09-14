package channel

import (
	"testing"
	"time"

	"github.com/titagaki/peercast-pcp/pcp"
)

type compatForwarder struct {
	mockOutput
	sid      pcp.GnuID
	received []*pcp.Atom
}

type compatEvictable struct {
	fakeRelayOutput
	sid pcp.GnuID
}

func (f *compatEvictable) PeerID() pcp.GnuID  { return f.sid }
func (f *compatEvictable) SendBcst(*pcp.Atom) {}

func TestCompatEvictionLocalAndUnproductive(t *testing.T) {
	ch := newTestChannel()
	local := &fakeRelayOutput{fakeOutput: fakeOutput{typ: OutputStreamPCP}, addr: "192.168.1.2:7144", firewalled: true}
	ch.AddOutput(local)
	if ch.EvictRelay() || local.evicted {
		t.Fatal("local relay evicted")
	}
	remote := &compatEvictable{fakeRelayOutput: fakeRelayOutput{fakeOutput: fakeOutput{typ: OutputStreamPCP}, addr: "203.0.113.2:7144"}, sid: pcp.GnuID{2}}
	ch.AddOutput(remote)
	if ch.EvictRelay() {
		t.Fatal("open relay without HOST evicted")
	}
	ch.ObserveHost(fullHostAtom(remote.sid, 0xcb007102, 0, 0, 0), remote.sid)
	if !ch.EvictRelay() || !remote.evicted || ch.NumRelays() != 1 {
		t.Fatal("unproductive relay slot not released")
	}
	if !ch.HasBanned("203.0.113.2") {
		t.Fatal("evicted node not banned")
	}
}

func (f *compatForwarder) PeerID() pcp.GnuID    { return f.sid }
func (f *compatForwarder) SendBcst(a *pcp.Atom) { f.received = append(f.received, a) }

func TestCompatBcstDirections(t *testing.T) {
	ch := newTestChannel()
	a := &compatForwarder{mockOutput: mockOutput{t: OutputStreamPCP, id: 1}, sid: pcp.GnuID{1}}
	b := &compatForwarder{mockOutput: mockOutput{t: OutputStreamPCP, id: 2}, sid: pcp.GnuID{2}}
	ch.AddOutput(a)
	ch.AddOutput(b)
	up := 0
	ch.SetUpstreamBcst(func(*pcp.Atom) { up++ })
	packet := func(group byte) *pcp.Atom {
		return pcp.NewParentAtom(pcp.PCPBcst, pcp.NewByteAtom(pcp.PCPBcstGroup, group))
	}
	ch.Broadcast(a, packet(pcp.PCPBcstGroupTrackers))
	if up != 1 || len(a.received) != 0 || len(b.received) != 0 {
		t.Fatal("TRACKERS routing")
	}
	ch.Broadcast(a, packet(pcp.PCPBcstGroupRelays))
	if up != 2 || len(a.received) != 0 || len(b.received) != 1 {
		t.Fatal("RELAYS routing")
	}
	ch.Broadcast(nil, packet(pcp.PCPBcstGroupRelays))
	if up != 2 || len(a.received) != 1 || len(b.received) != 2 {
		t.Fatal("upstream bounced or not delivered")
	}
	ch.Broadcast(a, packet(pcp.PCPBcstGroupRoot))
	if up != 2 || len(b.received) != 2 {
		t.Fatal("ROOT leaked into relay tree")
	}
}

func TestCompatReceivingAndAudioBoundaries(t *testing.T) {
	ch := newTestChannel()
	if ch.IsReceiving() {
		t.Fatal("empty channel receiving")
	}
	ch.Write([]byte("data"), 0, 0)
	if !ch.IsReceiving() {
		t.Fatal("fresh data not receiving")
	}
	ch.lastReceived.Store(time.Now().Add(-31 * time.Second).UnixNano())
	if ch.IsReceiving() || !ch.HasData() {
		t.Fatal("stale buffer must not imply receiving")
	}
	ch.Write([]byte("data"), 4, 0)
	ch.SourceDisconnected()
	if ch.IsReceiving() {
		t.Fatal("disconnected source receiving")
	}
	for _, flags := range []byte{4, 5} {
		ch.SetHeader([]byte{'F', 'L', 'V', 1, flags}, 0)
		if got := ch.CanStartContent(Content{ContFlags: 4}); got != (flags == 4) {
			t.Fatal("audio start boundary")
		}
		if ch.CanStartContent(Content{ContFlags: 5}) {
			t.Fatal("fragment accepted as start")
		}
	}
}

func TestCompatHostExpiryAndSubtreeRemoval(t *testing.T) {
	ch := newTestChannel()
	via := pcp.GnuID{1}
	child := pcp.GnuID{2}
	ch.ObserveHost(fullHostAtom(via, 1, 0, 2, 0), via)
	ch.ObserveHost(fullHostAtom(child, 2, 0, 3, 1), via)
	_, r := ch.nodes.totals()
	if r != 5 {
		t.Fatal("missing descendant statistics")
	}
	ch.RemoveNodeStats(via)
	if len(ch.KnownHosts()) != 0 {
		t.Fatal("disconnected subtree retained")
	}
	_, r = ch.nodes.totals()
	if r != 0 {
		t.Fatal("disconnected statistics retained")
	}
	ch.ObserveHost(fullHostAtom(child, 2, 0, 3, 1), via)
	ch.nodes.mu.Lock()
	ch.nodes.hostTimes[child] = time.Now().Add(-4 * time.Minute)
	s := ch.nodes.stats[child]
	s.Updated = time.Now().Add(-4 * time.Minute)
	ch.nodes.stats[child] = s
	ch.nodes.mu.Unlock()
	if len(ch.KnownHosts()) != 0 || len(ch.SelectSourceHosts(8, pcp.GnuID{}, 0)) != 0 {
		t.Fatal("expired host returned")
	}
	_, r = ch.nodes.totals()
	if r != 0 {
		t.Fatal("expired statistics counted")
	}
}

func TestCompatGlobalSlots(t *testing.T) {
	m := NewManager(pcp.GnuID{1})
	m.MaxRelaysTotal = 1
	m.IssueStreamKey("a", "a")
	m.IssueStreamKey("b", "b")
	a, err := m.Broadcast("a", ChannelInfo{}, TrackInfo{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := m.Broadcast("b", ChannelInfo{}, TrackInfo{})
	if err != nil {
		t.Fatal(err)
	}
	a.AddOutput(newPCPOut())
	if full, _ := b.SlotStatus(0, 0); !full {
		t.Fatal("global full not reflected on other channel")
	}
	b.AddOutput(newHTTPOut())
	if _, full := b.SlotStatus(0, 1); !full {
		t.Fatal("caller listener limit ignored")
	}
}
