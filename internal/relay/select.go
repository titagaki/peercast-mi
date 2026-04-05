package relay

import (
	"math/rand/v2"
)

// selectSourceHost picks the best connectable host from the source node list,
// using PeerCastStation's scoring algorithm.
//
// ourGlobalIP is our own external IPv4 (learned from the YP oleh); it is used
// to decide whether a source node is behind the same NAT as us. When it is,
// the node's LocalAddr is preferred and a site-local scoring bonus is applied,
// matching PeerCastStation's IsSiteLocal(Host) check
// (PCPSourceStream.cs: compares node.GlobalEndPoint to listener.GlobalEndPoint).
//
// Returns "" if no host is available.
func selectSourceHost(nodes []SourceNode, ignored *IgnoredNodeCollection, trackerAddr string, ourGlobalIP uint32) string {
	bestAddr := ""
	bestScore := -1.0

	for _, n := range nodes {
		if n.IsFirewalled {
			continue
		}

		// A node is "site-local" (same NAT as us) iff its external IP
		// matches our own learned global IP. When that holds we should
		// reach it via its LocalAddr; otherwise we must always go via
		// GlobalAddr, even if the node reports a LocalAddr (that private
		// IP belongs to a different LAN and would only time out).
		sameNAT := ourGlobalIP != 0 && n.GlobalIP != 0 && n.GlobalIP == ourGlobalIP

		var addr string
		if sameNAT && n.LocalAddr != "" {
			addr = n.LocalAddr
		} else {
			addr = n.GlobalAddr
		}
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
