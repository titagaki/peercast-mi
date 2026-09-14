package pcputil

import (
	"github.com/titagaki/peercast-pcp/pcp"
	"net"

	"github.com/titagaki/peercast-mi/internal/version"
)

// HostAtomParams holds the parameters needed to build a PCPHost atom.
type HostAtomParams struct {
	Network                                      *NetworkState
	LocalAddress, GlobalAddress, UpstreamAddress net.IP
	Firewalled                                   bool
	SessionID                                    pcp.GnuID
	LocalIP                                      uint32 // LAN IP (first ip/port pair in PCP Host atom)
	GlobalIP                                     uint32
	ListenPort                                   uint16
	ChannelID                                    pcp.GnuID
	NumListeners                                 int
	NumRelays                                    int
	Uptime                                       uint32
	OldPos                                       uint32
	NewPos                                       uint32
	IsTracker                                    bool
	HasGlobalIP                                  bool

	// RelayFull / DirectFull indicate that the corresponding slot is
	// exhausted, in which case the Relay / Direct flag1 bit is NOT set
	// (PeerCastStation 互換: slot 満杯時は該当 flag を落とす)。
	RelayFull  bool
	DirectFull bool

	// TrackerAtom adds an explicit pcp.PCPHostTracker atom (used in YP bcst).
	TrackerAtom bool

	// IsReceiving indicates that data is actually flowing (RTMP push active
	// or relay stream connected). When false the PCPHostFlags1Recv flag is
	// not set in the host atom. PeerCastStation 互換: Receiving フラグは
	// 実際にデータを受信しているときのみ立てる。
	IsReceiving bool

	// Optional upstream host info (for relay/YP bcst).
	UphostIP   uint32
	UphostPort uint16
	UphostHops uint32
}

// BuildHostAtom constructs a PCPHost atom from the given parameters.
//
// PeerCastStation expects two ip/port pairs in a Host atom: the first is
// interpreted as the global (public) endpoint and the second as the local
// (LAN) endpoint. HostPacket.BuildAtom() only emits one pair, so we
// build the atom manually here.
func BuildHostAtom(p HostAtomParams) *pcp.Atom {
	local, global, upstream := p.LocalAddress, p.GlobalAddress, p.UpstreamAddress
	if local == nil {
		local = pcp.IPv4FromUint32(p.LocalIP)
	}
	if global == nil {
		global = pcp.IPv4FromUint32(p.GlobalIP)
	}
	if upstream == nil {
		upstream = pcp.IPv4FromUint32(p.UphostIP)
	}
	if p.Network != nil {
		ip, known, open := p.Network.Status(local)
		if ip != nil {
			global = ip
		}
		p.Firewalled = !known || !open
		p.HasGlobalIP = global != nil && !global.IsUnspecified()
	}
	if ProtocolVersion(local) == 100 && (global == nil || global.IsUnspecified()) {
		global = net.IPv6zero
	}
	globalPort := p.ListenPort
	if p.Firewalled {
		globalPort = 0
	}
	flags := byte(pcp.PCPHostFlags1CIN)
	if p.Firewalled {
		flags |= pcp.PCPHostFlags1Push
	}
	if p.IsReceiving {
		flags |= pcp.PCPHostFlags1Recv
	}
	if !p.RelayFull {
		flags |= pcp.PCPHostFlags1Relay
	}
	if p.HasGlobalIP && !p.DirectFull {
		flags |= pcp.PCPHostFlags1Direct
	}
	if p.IsTracker {
		flags |= pcp.PCPHostFlags1Tracker
	}

	children := []*pcp.Atom{
		pcp.NewIDAtom(pcp.PCPHostID, p.SessionID),
		// 1st ip/port pair — global (public) endpoint
		IPAtom(pcp.PCPHostIP, global),
		pcp.NewShortAtom(pcp.PCPHostPort, globalPort),
		// 2nd ip/port pair — local (LAN) endpoint
		IPAtom(pcp.PCPHostIP, local),
		pcp.NewShortAtom(pcp.PCPHostPort, p.ListenPort),
		pcp.NewIntAtom(pcp.PCPHostNumListeners, uint32(p.NumListeners)),
		pcp.NewIntAtom(pcp.PCPHostNumRelays, uint32(p.NumRelays)),
		pcp.NewIntAtom(pcp.PCPHostUptime, p.Uptime),
		pcp.NewIntAtom(pcp.PCPHostOldPos, p.OldPos),
		pcp.NewIntAtom(pcp.PCPHostNewPos, p.NewPos),
		pcp.NewIDAtom(pcp.PCPHostChanID, p.ChannelID),
		pcp.NewByteAtom(pcp.PCPHostFlags1, flags),
		pcp.NewIntAtom(pcp.PCPHostVersion, version.PCPVersion),
		pcp.NewIntAtom(pcp.PCPHostVersionVP, version.PCPVersionVP),
		pcp.NewBytesAtom(pcp.PCPHostVersionExPrefix, []byte(version.ExPrefix)),
		pcp.NewShortAtom(pcp.PCPHostVersionExNumber, version.ExNumber()),
	}

	if p.TrackerAtom {
		children = append(children, pcp.NewIntAtom(pcp.PCPHostTracker, 1))
	}

	if !upstream.IsUnspecified() || p.UphostPort != 0 {
		children = append(children,
			IPAtom(pcp.PCPHostUphostIP, upstream),
			pcp.NewIntAtom(pcp.PCPHostUphostPort, uint32(p.UphostPort)),
		)
		if p.UphostHops != 0 {
			children = append(children, pcp.NewIntAtom(pcp.PCPHostUphostHops, p.UphostHops))
		}
	}

	return pcp.NewParentAtom(pcp.PCPHost, children...)
}
