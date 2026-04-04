package pcputil

import (
	"github.com/titagaki/peercast-pcp/pcp"

	"github.com/titagaki/peercast-mi/internal/version"
)

// HostAtomParams holds the parameters needed to build a PCPHost atom.
type HostAtomParams struct {
	SessionID    pcp.GnuID
	LocalIP      uint32 // LAN IP (first ip/port pair in PCP Host atom)
	GlobalIP     uint32
	ListenPort   uint16
	ChannelID    pcp.GnuID
	NumListeners int
	NumRelays    int
	Uptime       uint32
	OldPos       uint32
	NewPos       uint32
	IsTracker    bool
	HasGlobalIP  bool

	// TrackerAtom adds an explicit pcp.PCPHostTracker atom (used in YP bcst).
	TrackerAtom bool

	// Optional upstream host info (for relay/YP bcst).
	UphostIP   uint32
	UphostPort uint16
	UphostHops uint32
}

// BuildHostAtom constructs a PCPHost atom from the given parameters.
//
// PeerCastStation expects two ip/port pairs in a Host atom: the first is
// interpreted as the local (LAN) endpoint and the second as the global
// (public) endpoint. HostPacket.BuildAtom() only emits one pair, so we
// build the atom manually here.
func BuildHostAtom(p HostAtomParams) *pcp.Atom {
	flags := byte(pcp.PCPHostFlags1Relay | pcp.PCPHostFlags1Recv | pcp.PCPHostFlags1CIN)
	if p.HasGlobalIP {
		flags |= pcp.PCPHostFlags1Direct
	}
	if p.IsTracker {
		flags |= pcp.PCPHostFlags1Tracker
	}

	children := []*pcp.Atom{
		pcp.NewIDAtom(pcp.PCPHostID, p.SessionID),
		// 1st ip/port pair — local (LAN) endpoint
		pcp.NewIntAtom(pcp.PCPHostIP, p.LocalIP),
		pcp.NewShortAtom(pcp.PCPHostPort, p.ListenPort),
		// 2nd ip/port pair — global (public) endpoint
		pcp.NewIntAtom(pcp.PCPHostIP, p.GlobalIP),
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

	if p.UphostIP != 0 || p.UphostPort != 0 {
		children = append(children,
			pcp.NewIntAtom(pcp.PCPHostUphostIP, p.UphostIP),
			pcp.NewIntAtom(pcp.PCPHostUphostPort, uint32(p.UphostPort)),
		)
		if p.UphostHops != 0 {
			children = append(children, pcp.NewIntAtom(pcp.PCPHostUphostHops, p.UphostHops))
		}
	}

	return pcp.NewParentAtom(pcp.PCPHost, children...)
}
