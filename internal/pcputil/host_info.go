package pcputil

import "github.com/titagaki/peercast-pcp/pcp"

func HostID(a *pcp.Atom) (pcp.GnuID, bool) {
	if a == nil {
		return pcp.GnuID{}, false
	}
	if v := a.FindChild(pcp.PCPHostID); v != nil {
		id, err := v.GetID()
		return id, err == nil && !id.IsEmpty()
	}
	return pcp.GnuID{}, false
}

func Int(a *pcp.Atom, tag pcp.ID4) uint32 {
	if v := a.FindChild(tag); v != nil {
		n, _ := v.GetInt()
		return n
	}
	return 0
}

func Byte(a *pcp.Atom, tag pcp.ID4) byte {
	if v := a.FindChild(tag); v != nil {
		n, _ := v.GetByte()
		return n
	}
	return 0
}
