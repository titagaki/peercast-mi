package channel

import (
	"testing"

	"github.com/titagaki/peercast-pcp/pcp"
)

func hostAtom(sid pcp.GnuID) *pcp.Atom {
	return pcp.NewParentAtom(pcp.PCPHost, pcp.NewIDAtom(pcp.PCPHostID, sid))
}

func TestNodeTable_AddKnownHost_DedupAndOrder(t *testing.T) {
	var n nodeTable
	a := hostAtom(pcp.GnuID{1})
	b := hostAtom(pcp.GnuID{2})
	a2 := hostAtom(pcp.GnuID{1})

	n.addKnownHost(a)
	n.addKnownHost(b)
	n.addKnownHost(a2) // replaces a in place

	got := n.selectSourceHosts(8)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	// newest first: b was appended after a's slot
	if got[0] != b || got[1] != a2 {
		t.Fatal("expected [b, a2] (newest first, dedup in place)")
	}
}

func TestNodeTable_AddKnownHost_IgnoresInvalid(t *testing.T) {
	var n nodeTable
	n.addKnownHost(nil)
	n.addKnownHost(pcp.NewParentAtom(pcp.PCPHost)) // no sid
	n.addKnownHost(hostAtom(pcp.GnuID{}))          // zero sid
	if got := n.selectSourceHosts(8); len(got) != 0 {
		t.Fatalf("expected nothing cached, got %d", len(got))
	}
}

func TestNodeTable_KnownHostsBounded(t *testing.T) {
	var n nodeTable
	for i := 1; i <= maxKnownHosts+5; i++ {
		n.addKnownHost(hostAtom(pcp.GnuID{byte(i), byte(i >> 8)}))
	}
	if got := n.selectSourceHosts(1000); len(got) != maxKnownHosts {
		t.Fatalf("len = %d, want %d", len(got), maxKnownHosts)
	}
	if got := n.selectSourceHosts(3); len(got) != 3 {
		t.Fatalf("max not honoured: %d", len(got))
	}
	if got := n.selectSourceHosts(0); got != nil {
		t.Fatal("max <= 0 should return nil")
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
