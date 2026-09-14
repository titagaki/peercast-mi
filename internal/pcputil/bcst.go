package pcputil

import "github.com/titagaki/peercast-pcp/pcp"

// RouteBcst separates local delivery from forwarding. Payload processing is
// independent of TTL; a packet addressed to us must still be consumed at TTL 0.
func RouteBcst(a *pcp.Atom, self, channelID pcp.GnuID) (local bool, forwarded *pcp.Atom) {
	if a == nil || a.Tag != pcp.PCPBcst {
		return false, nil
	}
	if from := a.FindChild(pcp.PCPBcstFrom); from != nil {
		id, err := from.GetID()
		if err != nil || id == self {
			return false, nil
		}
	}
	if cid := a.FindChild(pcp.PCPBcstChanID); cid != nil {
		id, err := cid.GetID()
		if err != nil || (!id.IsEmpty() && id != channelID) {
			return false, nil
		}
	}
	local = true
	if dest := a.FindChild(pcp.PCPBcstDest); dest != nil {
		id, err := dest.GetID()
		if err != nil {
			return false, nil
		}
		if id == self {
			return true, nil
		}
		local = id.IsEmpty()
	}
	t, h, g, f := a.FindChild(pcp.PCPBcstTTL), a.FindChild(pcp.PCPBcstHops), a.FindChild(pcp.PCPBcstGroup), a.FindChild(pcp.PCPBcstFrom)
	if t == nil || h == nil || g == nil || f == nil {
		return local, nil
	}
	ttl, te := t.GetByte()
	hops, he := h.GetByte()
	_, ge := g.GetByte()
	if te != nil || he != nil || ge != nil || ttl <= 1 || hops == 255 {
		return local, nil
	}
	children := append([]*pcp.Atom(nil), a.Children()...)
	for i, c := range children {
		switch c.Tag {
		case pcp.PCPBcstTTL:
			children[i] = pcp.NewByteAtom(c.Tag, ttl-1)
		case pcp.PCPBcstHops:
			children[i] = pcp.NewByteAtom(c.Tag, hops+1)
		}
	}
	return local, pcp.NewParentAtom(pcp.PCPBcst, children...)
}
