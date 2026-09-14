package pcputil

import (
	"bytes"
	"net"
	"testing"

	"github.com/titagaki/peercast-pcp/pcp"
)

func TestCompatIPWireAndReachability(t *testing.T) {
	for _, addr := range []string{"203.0.113.9", "2001:db8::1234"} {
		ip := net.ParseIP(addr)
		a := IPAtom(pcp.PCPHeloRemoteIP, ip)
		if !AtomIP(a).Equal(ip) {
			t.Fatalf("round trip %s", addr)
		}
		if ip.To4() == nil && (a.Data()[0] != 0x34 || a.Data()[1] != 0x12 || a.Data()[15] != 0x20) {
			t.Fatal("IPv6 wire byte order")
		}
		n := new(NetworkState)
		if port, ping := n.HeloPort(ip, 7144); port != 0 || ping != 7144 {
			t.Fatal("unknown must probe")
		}
		for _, port := range []uint16{7144, 0} {
			n.Observe(pcp.NewParentAtom(pcp.PCPOleh, a, pcp.NewShortAtom(pcp.PCPHeloPort, port)))
			got, known, open := n.Status(ip)
			if !got.Equal(ip) || !known || open != (port != 0) {
				t.Fatal("reachability not observed")
			}
			h := BuildHostAtom(HostAtomParams{Network: n, LocalAddress: ip, ListenPort: 7144})
			p, _ := h.FindChildren(pcp.PCPHostPort)[0].GetShort()
			if p != port || (Byte(h, pcp.PCPHostFlags1)&pcp.PCPHostFlags1Push != 0) != (port == 0) {
				t.Fatal("HOST disagrees with observed status")
			}
		}
	}
}

func TestCompatBcstRouting(t *testing.T) {
	self, from, cid, other := pcp.GnuID{1}, pcp.GnuID{2}, pcp.GnuID{3}, pcp.GnuID{4}
	base := pcp.NewParentAtom(pcp.PCPBcst,
		pcp.NewIDAtom(pcp.PCPBcstFrom, from), pcp.NewIDAtom(pcp.PCPBcstChanID, cid),
		pcp.NewByteAtom(pcp.PCPBcstTTL, 3), pcp.NewByteAtom(pcp.PCPBcstHops, 0),
		pcp.NewByteAtom(pcp.PCPBcstGroup, pcp.PCPBcstGroupRelays))
	for _, tc := range []struct {
		name           string
		atom           *pcp.Atom
		local, forward bool
	}{
		{"broadcast", base, true, true},
		{"self TTL zero", ReplaceChild(ReplaceChild(base, pcp.NewIDAtom(pcp.PCPBcstDest, self)), pcp.NewByteAtom(pcp.PCPBcstTTL, 0)), true, false},
		{"other", ReplaceChild(base, pcp.NewIDAtom(pcp.PCPBcstDest, other)), false, true},
		{"TTL one", ReplaceChild(base, pcp.NewByteAtom(pcp.PCPBcstTTL, 1)), true, false},
		{"loop", ReplaceChild(base, pcp.NewIDAtom(pcp.PCPBcstFrom, self)), false, false},
		{"wrong channel", ReplaceChild(base, pcp.NewIDAtom(pcp.PCPBcstChanID, other)), false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := append([]byte(nil), tc.atom.FindChild(pcp.PCPBcstTTL).Data()...)
			local, f := RouteBcst(tc.atom, self, cid)
			if local != tc.local || (f != nil) != tc.forward {
				t.Fatalf("got local=%v forward=%v", local, f != nil)
			}
			if f != nil && (Byte(f, pcp.PCPBcstTTL) != 2 || Byte(f, pcp.PCPBcstHops) != 1) {
				t.Fatal("TTL/hops")
			}
			if !bytes.Equal(before, tc.atom.FindChild(pcp.PCPBcstTTL).Data()) {
				t.Fatal("input mutated")
			}
		})
	}
}
