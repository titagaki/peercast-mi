package relay

import (
	"math/rand/v2"
)

// sameNAT reports whether the node is behind the same NAT as us: its
// external IP matches our own learned global IP (ourGlobalIP, from the YP
// oleh). Unlike PeerCastStation's endpoint equality, this deliberately ignores
// ports: different nodes behind one NAT usually have different forwarded ports.
func (n SourceNode) sameNAT(ourGlobalIP uint32) bool {
	return ourGlobalIP != 0 && n.GlobalIP != 0 && n.GlobalIP == ourGlobalIP
}

// connectAddr returns the address to dial the node at: LocalAddr when it is
// behind the same NAT as us, otherwise GlobalAddr — even if the node reports
// a LocalAddr (that private IP belongs to a different LAN and would only
// time out). Returns "" if neither applies.
func (n SourceNode) connectAddr(ourGlobalIP uint32) string {
	if n.sameNAT(ourGlobalIP) && n.LocalAddr != "" {
		return n.LocalAddr
	}
	return n.GlobalAddr
}

// selectSourceHost picks the best connectable host from the source node list,
// using PeerCastStation's scoring algorithm. ourGlobalIP is used to prefer a
// node's LocalAddr and apply a site-local scoring bonus when it is behind the
// same NAT as us (see connectAddr).
//
// Returns "" if no host is available.
func selectSourceHost(nodes []SourceNode, ignored *IgnoredNodeCollection, trackerAddr string, ourGlobalIP uint32) string {
	bestAddr := ""
	bestScore := -1.0

	for _, n := range nodes {
		if n.IsFirewalled {
			continue
		}

		sameNAT := n.sameNAT(ourGlobalIP)
		addr := n.connectAddr(ourGlobalIP)
		if addr == "" {
			continue
		}
		if ignored.Contains(addr) {
			continue
		}

		// PeerCastStation scoring:
		//   (isSiteLocal ? 8000 : 0) + rand * (
		//     (isReceiving ? 4000 : 0) +
		//     (!isRelayFull ? 2000 : 0) +
		//     (max(10-hops, 0) * 100) +
		//     (relayCount * 10)
		//   )
		var score float64
		if sameNAT {
			score += 8000
		}

		var bonus float64
		if n.IsReceiving {
			bonus += 4000
		}
		if !n.IsRelayFull {
			bonus += 2000
		}
		hops := int(n.Hops)
		if hops < 10 {
			bonus += float64((10 - hops) * 100)
		}
		bonus += float64(n.RelayCount * 10)

		score += rand.Float64() * bonus

		if score > bestScore {
			bestScore = score
			bestAddr = addr
		}
	}

	if bestAddr != "" {
		return bestAddr
	}

	// Fall back to tracker if not ignored.
	if trackerAddr != "" && !ignored.Contains(trackerAddr) {
		return trackerAddr
	}
	return ""
}
