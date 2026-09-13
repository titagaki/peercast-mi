package channel

import (
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

// selectSourceHosts returns up to max known Host atoms, newest first.
func (n *nodeTable) selectSourceHosts(max int) []*pcp.Atom {
	if max <= 0 {
		return nil
	}
	n.mu.RLock()
	defer n.mu.RUnlock()
	count := len(n.knownHosts)
	if count > max {
		count = max
	}
	if count == 0 {
		return nil
	}
	out := make([]*pcp.Atom, 0, count)
	for i := len(n.knownHosts) - 1; i >= 0 && len(out) < count; i-- {
		out = append(out, n.knownHosts[i])
	}
	return out
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
