package channel

import (
	"github.com/titagaki/peercast-mi/internal/pcputil"
	"github.com/titagaki/peercast-pcp/pcp"
	"time"
)

// ObserveHost records a downstream report and the direct peer that delivered it.
func (c *Channel) ObserveHost(host *pcp.Atom, via pcp.GnuID) {
	id, ok := hostSessionID(host)
	if !ok {
		return
	}
	c.nodes.addKnownHost(host)
	c.nodes.updateStats(id, int(pcputil.Int(host, pcp.PCPHostNumListeners)), int(pcputil.Int(host, pcp.PCPHostNumRelays)))
	c.nodes.mu.Lock()
	if c.nodes.owners == nil {
		c.nodes.owners = make(map[pcp.GnuID]pcp.GnuID)
	}
	c.nodes.owners[id] = via
	c.nodes.mu.Unlock()
}

// KnownHosts returns a snapshot of fresh reports for relay-tree rendering.
func (c *Channel) KnownHosts() []*pcp.Atom {
	c.nodes.mu.RLock()
	defer c.nodes.mu.RUnlock()
	var hosts []*pcp.Atom
	for _, h := range c.nodes.knownHosts {
		id, _ := hostSessionID(h)
		if time.Since(c.nodes.hostTimes[id]) <= nodeLifetime {
			hosts = append(hosts, h)
		}
	}
	return hosts
}

func (c *Channel) relayUnproductive(id pcp.GnuID) bool {
	for _, h := range c.KnownHosts() {
		sid, _ := hostSessionID(h)
		if sid == id {
			return pcputil.Byte(h, pcp.PCPHostFlags1)&pcp.PCPHostFlags1Relay == 0 && pcputil.Int(h, pcp.PCPHostNumRelays) == 0
		}
	}
	return false
}
