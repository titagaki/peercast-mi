package channel

import (
	"fmt"
	"github.com/titagaki/peercast-pcp/pcp"
)

func (m *Manager) configureChannel(ch *Channel) {
	ch.Network = m.Network
	ch.slotCheck = func() (bool, bool) {
		bandwidthFull := m.MaxUpstreamKbps > 0 && m.TotalSendRate()*8/1000 >= int64(m.MaxUpstreamKbps)
		relayFull := m.MaxRelaysTotal > 0 && m.TotalRelays() >= m.MaxRelaysTotal
		return bandwidthFull || relayFull || ch.IsRelayFull(m.MaxRelays), bandwidthFull || ch.IsDirectFull(m.MaxListeners)
	}
}

// Reconnect preserves the active relay channel and its downstream connections.
func (m *Manager) Reconnect(id pcp.GnuID) error {
	m.mu.RLock()
	r := m.relays[id]
	m.mu.RUnlock()
	if r, ok := r.(interface{ Reconnect() }); ok {
		r.Reconnect()
		return nil
	}
	return fmt.Errorf("channel has no reconnectable relay source")
}
