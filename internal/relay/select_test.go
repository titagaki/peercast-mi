package relay

import (
	"net"
	"testing"
	"time"

	"github.com/titagaki/peercast-pcp/pcp"
)

func TestSelectSourceHost_PicksBestNode(t *testing.T) {
	ignored := NewIgnoredNodeCollection()

	nodes := []SourceNode{
		{SessionID: pcp.GnuID{1}, GlobalAddr: "1.2.3.4:7144", IsReceiving: false, IsRelayFull: true, Hops: 9},
		{SessionID: pcp.GnuID{2}, GlobalAddr: "5.6.7.8:7144", IsReceiving: true, IsRelayFull: false, Hops: 1},
	}

	// Node 1 max score: rand * (0 + 0 + 100 + 0) = ~100
	// Node 2 max score: rand * (4000 + 2000 + 900 + 0) = ~6900
	// Node 2 should win overwhelmingly.
	wins := map[string]int{}
	for i := 0; i < 100; i++ {
		addr := selectSourceHost(nodes, ignored, "tracker:7144", 0)
		wins[addr]++
	}
	if wins["5.6.7.8:7144"] < 90 {
		t.Errorf("expected node 2 to win most of the time, got %v", wins)
	}
}

func TestSelectSourceHost_SkipsIgnored(t *testing.T) {
	ignored := NewIgnoredNodeCollection()
	ignored.Add("1.2.3.4:7144")

	nodes := []SourceNode{
		{SessionID: pcp.GnuID{1}, GlobalAddr: "1.2.3.4:7144", IsReceiving: true},
		{SessionID: pcp.GnuID{2}, GlobalAddr: "5.6.7.8:7144", IsReceiving: true},
	}

	addr := selectSourceHost(nodes, ignored, "tracker:7144", 0)
	if addr != "5.6.7.8:7144" {
		t.Errorf("got %s, want 5.6.7.8:7144 (1.2.3.4 is ignored)", addr)
	}
}

func TestSelectSourceHost_SkipsFirewalled(t *testing.T) {
	ignored := NewIgnoredNodeCollection()

	nodes := []SourceNode{
		{SessionID: pcp.GnuID{1}, GlobalAddr: "1.2.3.4:7144", IsFirewalled: true, IsReceiving: true},
	}

	addr := selectSourceHost(nodes, ignored, "tracker:7144", 0)
	if addr != "tracker:7144" {
		t.Errorf("got %s, want tracker:7144 (firewalled node should be skipped)", addr)
	}
}

func TestSelectSourceHost_FallsBackToTracker(t *testing.T) {
	ignored := NewIgnoredNodeCollection()

	addr := selectSourceHost(nil, ignored, "tracker:7144", 0)
	if addr != "tracker:7144" {
		t.Errorf("got %s, want tracker:7144", addr)
	}
}

func TestSelectSourceHost_ReturnsEmptyWhenAllIgnored(t *testing.T) {
	ignored := NewIgnoredNodeCollection()
	ignored.Add("1.2.3.4:7144")
	ignored.Add("tracker:7144")

	nodes := []SourceNode{
		{SessionID: pcp.GnuID{1}, GlobalAddr: "1.2.3.4:7144", IsReceiving: true},
	}

	addr := selectSourceHost(nodes, ignored, "tracker:7144", 0)
	if addr != "" {
		t.Errorf("got %s, want empty (all ignored)", addr)
	}
}

func TestSelectSourceHost_UsesLocalAddrWhenSameNAT(t *testing.T) {
	ignored := NewIgnoredNodeCollection()
	ourIP, _ := pcp.IPv4ToUint32(net.IPv4(203, 0, 113, 5))

	// Node reports the same global IP as ours -> behind the same NAT ->
	// must be reached via LocalAddr.
	nodes := []SourceNode{
		{
			SessionID:   pcp.GnuID{1},
			GlobalIP:    ourIP,
			GlobalAddr:  "203.0.113.5:7144",
			LocalAddr:   "192.168.0.7:7144",
			IsReceiving: true,
		},
	}

	addr := selectSourceHost(nodes, ignored, "tracker:7144", ourIP)
	if addr != "192.168.0.7:7144" {
		t.Errorf("got %s, want 192.168.0.7:7144 (same NAT -> LocalAddr)", addr)
	}
}

func TestSelectSourceHost_UsesGlobalAddrWhenDifferentNAT(t *testing.T) {
	ignored := NewIgnoredNodeCollection()
	ourIP, _ := pcp.IPv4ToUint32(net.IPv4(203, 0, 113, 5))
	theirIP, _ := pcp.IPv4ToUint32(net.IPv4(198, 51, 100, 9))

	// Node reports a different global IP -> different LAN -> must use
	// GlobalAddr even though it advertises a private LocalAddr. This is
	// the regression that caused dial timeouts to 192.168.0.7.
	nodes := []SourceNode{
		{
			SessionID:   pcp.GnuID{1},
			GlobalIP:    theirIP,
			GlobalAddr:  "198.51.100.9:7144",
			LocalAddr:   "192.168.0.7:7144",
			IsReceiving: true,
		},
	}

	addr := selectSourceHost(nodes, ignored, "tracker:7144", ourIP)
	if addr != "198.51.100.9:7144" {
		t.Errorf("got %s, want 198.51.100.9:7144 (different NAT -> GlobalAddr)", addr)
	}
}

func TestSelectSourceHost_UsesGlobalAddrWhenOurIPUnknown(t *testing.T) {
	ignored := NewIgnoredNodeCollection()

	// Before the YP oleh is received we don't know our own global IP;
	// fall back to GlobalAddr for every node rather than guessing the
	// LAN from RFC1918 ranges.
	nodes := []SourceNode{
		{
			SessionID:   pcp.GnuID{1},
			GlobalIP:    0xC6336405, // 198.51.100.5
			GlobalAddr:  "198.51.100.5:7144",
			LocalAddr:   "192.168.0.7:7144",
			IsReceiving: true,
		},
	}

	addr := selectSourceHost(nodes, ignored, "tracker:7144", 0)
	if addr != "198.51.100.5:7144" {
		t.Errorf("got %s, want 198.51.100.5:7144 (unknown global IP -> GlobalAddr)", addr)
	}
}

func TestIgnoredNodeCollection_Expiry(t *testing.T) {
	c := &IgnoredNodeCollection{
		entries:   make(map[string]time.Time),
		threshold: 50 * time.Millisecond,
	}

	c.Add("1.2.3.4:7144")
	if !c.Contains("1.2.3.4:7144") {
		t.Error("should contain just-added entry")
	}

	time.Sleep(60 * time.Millisecond)

	if c.Contains("1.2.3.4:7144") {
		t.Error("should not contain expired entry")
	}
}

func TestIgnoredNodeCollection_NotContained(t *testing.T) {
	c := NewIgnoredNodeCollection()
	if c.Contains("1.2.3.4:7144") {
		t.Error("should not contain unknown entry")
	}
}
