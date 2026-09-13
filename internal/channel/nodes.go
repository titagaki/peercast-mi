package channel

import (
	"math/rand/v2"
	"sort"
	"sync"

	"github.com/titagaki/peercast-pcp/pcp"
)

const maxKnownHosts = 32

// nodeStats holds the listener/relay counts reported by a downstream node.
type nodeStats struct {
	Listeners int
	Relays    int
}

// nodeTable tracks what a channel has learned about other nodes through
// BCST HOST atoms: a bounded cache of Host atoms usable as alternative relay
// candidates, and per-downstream-node listener/relay counts.
// PeerCastStation 互換: Channel.Nodes / SelectSourceHosts / AddNode。
//
// It has its own mutex and is a leaf: it never calls back into Channel.
type nodeTable struct {
	mu sync.RWMutex

	// knownHosts is a bounded cache of Host atoms observed via bcst
	// forwarding, deduped by session ID, oldest first.
	knownHosts []*pcp.Atom

	// stats maps a downstream node's session ID to the counts it reported.
	stats map[pcp.GnuID]nodeStats
}

// addKnownHost records a Host atom, replacing any existing entry with the
// same session ID. Atoms without a usable session ID are ignored. Older
// entries are evicted when the cache is full.
func (n *nodeTable) addKnownHost(host *pcp.Atom) {
	sid, ok := hostSessionID(host)
	if !ok {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	for i, h := range n.knownHosts {
		if id, ok := hostSessionID(h); ok && id == sid {
			n.knownHosts[i] = host
			return
		}
	}
	if len(n.knownHosts) >= maxKnownHosts {
		n.knownHosts = n.knownHosts[1:]
	}
	n.knownHosts = append(n.knownHosts, host)
}

// selectSourceHosts returns up to max known Host atoms as alternative relay
// candidates for the requester, best first. The requester's own Host atom is
// excluded. Ranking follows PeerCastStation's SelectSourceHosts: a global
// endpoint, the requester's own address, free relay slots, receiving, fewer
// hops and more relays score higher, with a random tie-breaker.
func (n *nodeTable) selectSourceHosts(max int, requester pcp.GnuID, requesterIP uint32) []*pcp.Atom {
	if max <= 0 {
		return nil
	}
	n.mu.RLock()
	defer n.mu.RUnlock()
	type scored struct {
		atom  *pcp.Atom
		score float64
	}
	cands := make([]scored, 0, len(n.knownHosts))
	for _, h := range n.knownHosts {
		if sid, ok := hostSessionID(h); ok && sid == requester {
			continue
		}
		cands = append(cands, scored{h, hostScore(h, requesterIP)})
	}
	if len(cands) == 0 {
		return nil
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].score > cands[j].score })
	if len(cands) > max {
		cands = cands[:max]
	}
	out := make([]*pcp.Atom, len(cands))
	for i, c := range cands {
		out[i] = c.atom
	}
	return out
}

// hostScore ranks a Host atom for selectSourceHosts.
// PeerCastStation 互換 (PCPOutputStream.SelectSourceHosts):
//
//	(GlobalEndPoint あり ? 16000 : 0) + (要求元と同じ IP ? 8000 : 0) +
//	(!IsRelayFull ? 4000 : 0) + (IsReceiving ? 2000 : 0) +
//	max(10-Hops, 0)*100 + RelayCount*10 + rand
//
// The first ip/port pair is the global endpoint; IsRelayFull is the Relay
// flag being clear; Hops is the uphp atom.
func hostScore(host *pcp.Atom, requesterIP uint32) float64 {
	var (
		ip            uint32
		hasIP, hasPrt bool
		flags         byte
		relays, hops  uint32
	)
	for _, c := range host.Children() {
		switch c.Tag {
		case pcp.PCPHostIP:
			if !hasIP {
				ip, _ = c.GetInt()
				hasIP = true
			}
		case pcp.PCPHostPort:
			hasPrt = true
		case pcp.PCPHostFlags1:
			flags, _ = c.GetByte()
		case pcp.PCPHostNumRelays:
			relays, _ = c.GetInt()
		case pcp.PCPHostUphostHops:
			hops, _ = c.GetInt()
		}
	}
	hasGlobal := hasIP && hasPrt
	var score float64
	if hasGlobal {
		score += 16000
	}
	if hasGlobal && ip == requesterIP {
		score += 8000
	}
	if flags&pcp.PCPHostFlags1Relay != 0 {
		score += 4000
	}
	if flags&pcp.PCPHostFlags1Recv != 0 {
		score += 2000
	}
	if hops < 10 {
		score += float64((10 - hops) * 100)
	}
	score += float64(relays * 10)
	return score + rand.Float64()
}

// updateStats records the counts reported by a downstream node.
func (n *nodeTable) updateStats(sessionID pcp.GnuID, listeners, relays int) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.stats == nil {
		n.stats = make(map[pcp.GnuID]nodeStats)
	}
	n.stats[sessionID] = nodeStats{Listeners: listeners, Relays: relays}
}

// removeStats forgets a downstream node's counts.
func (n *nodeTable) removeStats(sessionID pcp.GnuID) {
	n.mu.Lock()
	defer n.mu.Unlock()
	delete(n.stats, sessionID)
}

// totals returns the sum of all downstream nodes' listener and relay counts.
func (n *nodeTable) totals() (listeners, relays int) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	for _, s := range n.stats {
		listeners += s.Listeners
		relays += s.Relays
	}
	return listeners, relays
}

// reset drops all cached hosts and stats.
func (n *nodeTable) reset() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.knownHosts = nil
	n.stats = nil
}

// hostSessionID extracts the (non-zero) session ID from a Host atom.
func hostSessionID(host *pcp.Atom) (pcp.GnuID, bool) {
	if host == nil {
		return pcp.GnuID{}, false
	}
	sidAtom := host.FindChild(pcp.PCPHostID)
	if sidAtom == nil {
		return pcp.GnuID{}, false
	}
	sid, err := sidAtom.GetID()
	if err != nil || sid == (pcp.GnuID{}) {
		return pcp.GnuID{}, false
	}
	return sid, true
}
