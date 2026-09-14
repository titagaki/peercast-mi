package pcputil

import (
	"net"
	"sync"

	"github.com/titagaki/peercast-pcp/pcp"
)

// Address atoms store IPv4 integers and reversed IPv6 bytes on the wire.
func IPAtom(tag pcp.ID4, ip net.IP) *pcp.Atom {
	if v4 := ip.To4(); v4 != nil {
		n, _ := pcp.IPv4ToUint32(v4)
		return pcp.NewIntAtom(tag, n)
	}
	b := append([]byte(nil), ip.To16()...)
	if len(b) == 0 {
		return pcp.NewIntAtom(tag, 0)
	}
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return pcp.NewBytesAtom(tag, b)
}

func AtomIP(a *pcp.Atom) net.IP {
	if a == nil {
		return nil
	}
	if len(a.Data()) == 4 {
		n, _ := a.GetInt()
		return pcp.IPv4FromUint32(n)
	}
	if len(a.Data()) != 16 {
		return nil
	}
	b := append(net.IP(nil), a.Data()...)
	for i, j := 0, 15; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return b
}

func AddrIP(addr net.Addr) net.IP {
	if a, ok := addr.(*net.TCPAddr); ok {
		return a.IP
	}
	return nil
}

func ProtocolVersion(ip net.IP) uint32 {
	if ip != nil && ip.To4() == nil {
		return 100
	}
	return 1
}

// ParseHelo handles rip independently because peercast-pcp v0.3.1 only
// understands a uint32 rip. All other fields keep the library's validation.
func ParseHelo(a *pcp.Atom) (pcp.HeloPacket, error) {
	var children []*pcp.Atom
	for _, c := range a.Children() {
		if c.Tag != pcp.PCPHeloRemoteIP {
			children = append(children, c)
		}
	}
	h, err := pcp.ParseHeloPacket(pcp.NewParentAtom(a.Tag, children...))
	if ip := AtomIP(a.FindChild(pcp.PCPHeloRemoteIP)); ip.To4() != nil {
		h.RemoteIP, _ = pcp.IPv4ToUint32(ip)
	}
	return h, err
}

// ReplaceChild returns a new immutable atom, replacing all occurrences of tag.
func ReplaceChild(a *pcp.Atom, child *pcp.Atom) *pcp.Atom {
	var children []*pcp.Atom
	for _, c := range a.Children() {
		if c.Tag != child.Tag {
			children = append(children, c)
		}
	}
	return pcp.NewParentAtom(a.Tag, append(children, child)...)
}

type networkStatus struct {
	ip          net.IP
	known, open bool
}

// NetworkState shares the latest observed address and port status per family.
type NetworkState struct {
	mu     sync.RWMutex
	v4, v6 networkStatus
}

func (n *NetworkState) Observe(oleh *pcp.Atom) {
	if n == nil {
		return
	}
	ip := AtomIP(oleh.FindChild(pcp.PCPHeloRemoteIP))
	if ip == nil || ip.IsUnspecified() {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	s := &n.v4
	if ip.To4() == nil {
		s = &n.v6
	}
	s.ip = append(net.IP(nil), ip...)
	if p := oleh.FindChild(pcp.PCPHeloPort); p != nil {
		port, err := p.GetShort()
		if err == nil {
			s.known, s.open = true, port != 0
		}
	}
}

func (n *NetworkState) Status(local net.IP) (ip net.IP, known, open bool) {
	if n == nil {
		return nil, false, false
	}
	n.mu.RLock()
	defer n.mu.RUnlock()
	s := n.v4
	if ProtocolVersion(local) == 100 {
		s = n.v6
	}
	return append(net.IP(nil), s.ip...), s.known, s.open
}

func (n *NetworkState) HeloPort(local net.IP, listen uint16) (port, ping uint16) {
	_, known, open := n.Status(local)
	if !known {
		return 0, listen
	}
	if open {
		return listen, 0
	}
	return 0, 0
}
