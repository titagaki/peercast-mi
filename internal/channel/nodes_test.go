package channel

import (
	"testing"
	"time"

	"github.com/titagaki/peercast-pcp/pcp"
)

func hostAtom(sid pcp.GnuID) *pcp.Atom {
	return pcp.NewParentAtom(pcp.PCPHost, pcp.NewIDAtom(pcp.PCPHostID, sid))
}

// fullHostAtom builds a Host atom with the fields hostScore looks at.
func fullHostAtom(sid pcp.GnuID, ip uint32, flags byte, relays, hops uint32) *pcp.Atom {
	return pcp.NewParentAtom(pcp.PCPHost,
		pcp.NewIDAtom(pcp.PCPHostID, sid),
		pcp.NewIntAtom(pcp.PCPHostIP, ip),
		pcp.NewShortAtom(pcp.PCPHostPort, 7144),
		pcp.NewIntAtom(pcp.PCPHostNumRelays, relays),
		pcp.NewByteAtom(pcp.PCPHostFlags1, flags),
		pcp.NewIntAtom(pcp.PCPHostUphostHops, hops),
	)
}

func selectAll(n *nodeTable, max int) []*pcp.Atom {
	return n.selectSourceHosts(max, pcp.GnuID{}, 0)
}

func TestNodeTable_AddKnownHost_DedupAndOrder(t *testing.T) {
	var n nodeTable
	a := hostAtom(pcp.GnuID{1})
	b := hostAtom(pcp.GnuID{2})
	a2 := hostAtom(pcp.GnuID{1})

	n.addKnownHost(a)
	n.addKnownHost(b)
	n.addKnownHost(a2) // replaces a in place

	got := selectAll(&n, 8)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if (got[0] != b || got[1] != a2) && (got[0] != a2 || got[1] != b) {
		t.Fatal("expected {b, a2} (dedup in place, a replaced by a2)")
	}
}

func TestNodeTable_AddKnownHost_IgnoresInvalid(t *testing.T) {
	var n nodeTable
	n.addKnownHost(nil)
	n.addKnownHost(pcp.NewParentAtom(pcp.PCPHost)) // no sid
	n.addKnownHost(hostAtom(pcp.GnuID{}))          // zero sid
	if got := selectAll(&n, 8); len(got) != 0 {
		t.Fatalf("expected nothing cached, got %d", len(got))
	}
}

func TestNodeTable_KnownHostsBounded(t *testing.T) {
	var n nodeTable
	for i := 1; i <= maxKnownHosts+5; i++ {
		n.addKnownHost(hostAtom(pcp.GnuID{byte(i), byte(i >> 8)}))
	}
	if got := selectAll(&n, 1000); len(got) != maxKnownHosts {
		t.Fatalf("len = %d, want %d", len(got), maxKnownHosts)
	}
	if got := selectAll(&n, 3); len(got) != 3 {
		t.Fatalf("max not honoured: %d", len(got))
	}
	if got := selectAll(&n, 0); got != nil {
		t.Fatal("max <= 0 should return nil")
	}
}

func TestNodeTable_SelectSourceHosts_ExcludesRequester(t *testing.T) {
	var n nodeTable
	n.addKnownHost(hostAtom(pcp.GnuID{1}))
	n.addKnownHost(hostAtom(pcp.GnuID{2}))
	got := n.selectSourceHosts(8, pcp.GnuID{1}, 0)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if sid, _ := hostSessionID(got[0]); sid != (pcp.GnuID{2}) {
		t.Fatalf("requester's own host must be excluded, got %v", sid)
	}
}

func TestNodeTable_SelectSourceHosts_Ranking(t *testing.T) {
	const (
		relay = pcp.PCPHostFlags1Relay
		recv  = pcp.PCPHostFlags1Recv
	)
	// Inserted worst-first so that insertion order alone would be wrong.
	noEndpoint := hostAtom(pcp.GnuID{1})
	relayFull := fullHostAtom(pcp.GnuID{2}, 0x0a000002, recv, 0, 1)
	free := fullHostAtom(pcp.GnuID{3}, 0x0a000003, relay|recv, 0, 1)
	sameIP := fullHostAtom(pcp.GnuID{4}, 0xc0a80001, 0, 0, 1)
	var n nodeTable
	for _, h := range []*pcp.Atom{noEndpoint, relayFull, free, sameIP} {
		n.addKnownHost(h)
	}
	got := n.selectSourceHosts(8, pcp.GnuID{}, 0xc0a80001)
	want := []*pcp.Atom{sameIP, free, relayFull, noEndpoint}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			gs, _ := hostSessionID(got[i])
			ws, _ := hostSessionID(want[i])
			t.Fatalf("rank %d = %v, want %v", i, gs[0], ws[0])
		}
	}
}

func TestHostScore_HopsAndRelays(t *testing.T) {
	base := hostScore(fullHostAtom(pcp.GnuID{1}, 1, 0, 0, 0), 0)
	fewerHops := hostScore(fullHostAtom(pcp.GnuID{1}, 1, 0, 0, 3), 0)
	if base-fewerHops < 299 || base-fewerHops > 301 {
		t.Fatalf("3 hops should cost ~300, got %f", base-fewerHops)
	}
	farAway := hostScore(fullHostAtom(pcp.GnuID{1}, 1, 0, 0, 50), 0)
	if base-farAway < 999 || base-farAway > 1001 {
		t.Fatalf("hops beyond 10 should be capped at 1000, got %f", base-farAway)
	}
	moreRelays := hostScore(fullHostAtom(pcp.GnuID{1}, 1, 0, 5, 0), 0)
	if moreRelays-base < 49 || moreRelays-base > 51 {
		t.Fatalf("5 relays should add ~50, got %f", moreRelays-base)
	}
}

func TestChannel_Ban(t *testing.T) {
	ch := New(pcp.GnuID{1}, pcp.GnuID{}, 0)
	if ch.HasBanned("10.0.0.1") {
		t.Fatal("nothing banned yet")
	}
	ch.Ban("10.0.0.1", time.Now().Add(time.Minute))
	if !ch.HasBanned("10.0.0.1") {
		t.Fatal("should be banned")
	}
	if ch.HasBanned("10.0.0.2") {
		t.Fatal("other key must not be banned")
	}
	ch.Ban("10.0.0.1", time.Now().Add(-time.Second))
	if ch.HasBanned("10.0.0.1") {
		t.Fatal("expired ban must not count")
	}
}

type fakeRelayOutput struct {
	fakeOutput
	addr       string
	firewalled bool
	evicted    bool
}

func (f *fakeRelayOutput) RemoteAddr() string { return f.addr }
func (f *fakeRelayOutput) IsFirewalled() bool { return f.firewalled }
func (f *fakeRelayOutput) Evict()             { f.evicted = true }

func TestChannel_MakeRelayable_EvictsAndBans(t *testing.T) {
	ch := New(pcp.GnuID{1}, pcp.GnuID{}, 0)
	open := &fakeRelayOutput{fakeOutput: fakeOutput{typ: OutputStreamPCP}, addr: "10.0.0.1:7144"}
	fw := &fakeRelayOutput{fakeOutput: fakeOutput{typ: OutputStreamPCP}, addr: "10.0.0.2:51000", firewalled: true}
	ch.AddOutput(open)
	ch.AddOutput(fw)

	if !ch.MakeRelayable(0) {
		t.Fatal("unlimited relays must always be relayable")
	}
	if !ch.MakeRelayable(3) {
		t.Fatal("free slot must be relayable without eviction")
	}
	if open.evicted || fw.evicted {
		t.Fatal("nothing should be evicted while a slot is free")
	}

	if !ch.MakeRelayable(2) {
		t.Fatal("should free a slot by evicting the firewalled node")
	}
	if !fw.evicted || fw.closed {
		t.Fatal("firewalled node must be evicted via Evict(), not Close()")
	}
	if open.evicted {
		t.Fatal("node with an open port must not be evicted")
	}
	if !ch.HasBanned("10.0.0.2") {
		t.Fatal("evicted node's IP must be banned")
	}
	if ch.HasBanned("10.0.0.1") {
		t.Fatal("remaining node must not be banned")
	}

	// Only non-evictable nodes left: cannot make room.
	ch.RemoveOutput(fw)
	ch.AddOutput(&fakeRelayOutput{fakeOutput: fakeOutput{typ: OutputStreamPCP}, addr: "10.0.0.3:7144"})
	if ch.MakeRelayable(2) {
		t.Fatal("no firewalled node to evict: must report full")
	}
}

func TestNodeTable_Stats(t *testing.T) {
	var n nodeTable
	if l, r := n.totals(); l != 0 || r != 0 {
		t.Fatal("empty table should sum to zero")
	}
	n.updateStats(pcp.GnuID{1}, 2, 1)
	n.updateStats(pcp.GnuID{2}, 3, 0)
	n.updateStats(pcp.GnuID{1}, 5, 1) // overwrite
	if l, r := n.totals(); l != 8 || r != 1 {
		t.Fatalf("totals = (%d, %d), want (8, 1)", l, r)
	}
	n.removeStats(pcp.GnuID{1})
	if l, r := n.totals(); l != 3 || r != 0 {
		t.Fatalf("totals after remove = (%d, %d), want (3, 0)", l, r)
	}
	n.reset()
	if l, _ := n.totals(); l != 0 {
		t.Fatal("reset should clear stats")
	}
}

func TestChannel_TotalsIncludeDownstream(t *testing.T) {
	ch := New(pcp.GnuID{1}, pcp.GnuID{}, 0)
	ch.AddOutput(&fakeOutput{typ: OutputStreamHTTP})
	ch.AddOutput(&fakeOutput{typ: OutputStreamPCP})
	ch.UpdateNodeStats(pcp.GnuID{9}, 4, 2)
	if ch.TotalListeners() != 5 || ch.TotalRelays() != 3 {
		t.Fatalf("totals = (%d, %d), want (5, 3)", ch.TotalListeners(), ch.TotalRelays())
	}
	ch.CloseAll()
	if ch.TotalListeners() != 0 || ch.TotalRelays() != 0 {
		t.Fatal("CloseAll should reset downstream stats")
	}
}
