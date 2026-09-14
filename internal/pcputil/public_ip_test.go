package pcputil

import (
	"github.com/titagaki/peercast-pcp/pcp"
	"net"
	"testing"
)

func TestPublicIPv4OverridesDockerObservation(t *testing.T) {
	n := new(NetworkState)
	public := net.ParseIP("153.127.50.98")
	local := net.ParseIP("172.20.0.4")
	n.SetPublicIPv4(public)
	for _, port := range []uint16{7154, 0, 7154} {
		n.Observe(pcp.NewParentAtom(pcp.PCPOleh, IPAtom(pcp.PCPHeloRemoteIP, local), pcp.NewShortAtom(pcp.PCPHeloPort, port)))
		host := BuildHostAtom(HostAtomParams{Network: n, LocalAddress: local, ListenPort: 7154, IsReceiving: true, IsTracker: true})
		if ip := AtomIP(host.FindChild(pcp.PCPHostIP)); !ip.Equal(public) {
			t.Fatal("advertised internal IP", ip)
		}
		actual, _ := host.FindChild(pcp.PCPHostPort).GetShort()
		if actual != port {
			t.Fatal("port check overridden", actual, port)
		}
		flags := Byte(host, pcp.PCPHostFlags1)
		if (flags&pcp.PCPHostFlags1Push != 0) != (port == 0) {
			t.Fatal("firewall flag lost")
		}
	}
	v6 := net.ParseIP("2001:db8::1")
	n.Observe(pcp.NewParentAtom(pcp.PCPOleh, IPAtom(pcp.PCPHeloRemoteIP, v6), pcp.NewShortAtom(pcp.PCPHeloPort, 7154)))
	if ip, _, _ := n.Status(v6); !ip.Equal(v6) {
		t.Fatal("IPv6 overwritten", ip)
	}
	n.SetPublicIPv4(nil)
	if ip, _, _ := n.Status(local); !ip.Equal(local) {
		t.Fatal("automatic discovery not restored", ip)
	}
}
