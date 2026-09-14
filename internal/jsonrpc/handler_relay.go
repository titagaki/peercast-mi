package jsonrpc

import (
	"net"
	"sort"
	"strconv"

	"github.com/titagaki/peercast-mi/internal/channel"
	"github.com/titagaki/peercast-mi/internal/pcputil"
	"github.com/titagaki/peercast-mi/internal/version"
	"github.com/titagaki/peercast-pcp/pcp"
)

type relayTreeNode struct {
	SessionID     string          `json:"sessionId"`
	Address       string          `json:"address"`
	Port          int             `json:"port"`
	IsFirewalled  bool            `json:"isFirewalled"`
	LocalRelays   int             `json:"localRelays"`
	LocalDirects  int             `json:"localDirects"`
	IsTracker     bool            `json:"isTracker"`
	IsRelayFull   bool            `json:"isRelayFull"`
	IsDirectFull  bool            `json:"isDirectFull"`
	IsReceiving   bool            `json:"isReceiving"`
	IsControlFull bool            `json:"isControlFull"`
	Version       int             `json:"version"`
	VersionString string          `json:"versionString"`
	Children      []relayTreeNode `json:"children"`
}

func (s *Server) getChannelRelayTree(ch *channel.Channel) (interface{}, *rpcError) {
	nodes := make(map[string]relayTreeNode)
	parents := make(map[string]string)
	endpoints := make(map[string]string)
	self := gnuIDString(s.sessionID)
	for _, rn := range ch.RelayNodes() {
		host, _, _ := net.SplitHostPort(rn.RemoteAddr)
		id := gnuIDString(rn.SessionID)
		nodes[id] = relayTreeNode{SessionID: id, Address: host, Port: int(rn.RemotePort), IsFirewalled: rn.IsFirewalled, Version: int(rn.Version), VersionString: rn.Agent}
		parents[id] = self
	}
	hosts := ch.KnownHosts()
	for _, h := range hosts {
		sid, ok := pcputil.HostID(h)
		if !ok || sid == s.sessionID {
			continue
		}
		id := gnuIDString(sid)
		n := nodes[id]
		n.SessionID = id
		flags := pcputil.Byte(h, pcp.PCPHostFlags1)
		n.IsFirewalled = flags&pcp.PCPHostFlags1Push != 0
		n.IsTracker = flags&pcp.PCPHostFlags1Tracker != 0
		n.IsRelayFull = flags&pcp.PCPHostFlags1Relay == 0
		n.IsDirectFull = flags&pcp.PCPHostFlags1Direct == 0
		n.IsReceiving = flags&pcp.PCPHostFlags1Recv != 0
		n.IsControlFull = flags&pcp.PCPHostFlags1CIN == 0
		n.LocalRelays = int(pcputil.Int(h, pcp.PCPHostNumRelays))
		n.LocalDirects = int(pcputil.Int(h, pcp.PCPHostNumListeners))
		n.Version = int(pcputil.Int(h, pcp.PCPHostVersion))
		ips, ports := h.FindChildren(pcp.PCPHostIP), h.FindChildren(pcp.PCPHostPort)
		for i := 0; i < len(ips) && i < len(ports); i++ {
			ip := pcputil.AtomIP(ips[i])
			port, _ := ports[i].GetShort()
			if ip == nil || ip.IsUnspecified() || port == 0 {
				continue
			}
			endpoints[net.JoinHostPort(ip.String(), strconv.Itoa(int(port)))] = id
			if i == 0 || n.Address == "" {
				n.Address, n.Port = ip.String(), int(port)
			}
		}
		if ex := h.FindChild(pcp.PCPHostVersionExPrefix); ex != nil {
			if num := h.FindChild(pcp.PCPHostVersionExNumber); num != nil {
				v, _ := num.GetShort()
				n.VersionString = string(ex.Data()) + strconv.Itoa(int(v))
			}
		}
		nodes[id] = n
	}
	for _, h := range hosts {
		sid, _ := pcputil.HostID(h)
		id := gnuIDString(sid)
		if _, direct := parents[id]; direct {
			continue
		}
		up := pcputil.AtomIP(h.FindChild(pcp.PCPHostUphostIP))
		port := pcputil.Int(h, pcp.PCPHostUphostPort)
		parents[id] = endpoints[net.JoinHostPort(up.String(), strconv.Itoa(int(port)))]
		if parents[id] == "" {
			parents[id] = self
		}
	}
	// Deterministic order, bounded traversal, and a visited set make malformed
	// or cyclic reports harmless. Orphans remain visible under our node.
	ids := make([]string, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	visited := map[string]bool{self: true}
	var children func(string) []relayTreeNode
	children = func(parent string) []relayTreeNode {
		result := []relayTreeNode{}
		for _, id := range ids {
			if visited[id] || parents[id] != parent {
				continue
			}
			visited[id] = true
			n := nodes[id]
			n.Children = children(id)
			result = append(result, n)
		}
		return result
	}
	r, d := ch.SlotStatus(s.cfg.MaxRelays, s.cfg.MaxListeners)
	upstreamHost, _, _ := net.SplitHostPort(ch.UpstreamAddr())
	ip, known, open := ch.Network.Status(net.ParseIP(upstreamHost))
	n := relayTreeNode{SessionID: self, Port: s.cfg.PeercastPort, IsFirewalled: ch.Network != nil && (!known || !open), LocalRelays: ch.NumRelays(), LocalDirects: ch.NumListeners(), IsTracker: ch.IsBroadcasting(), IsRelayFull: r, IsDirectFull: d, IsReceiving: ch.IsReceiving(), Version: version.PCPVersion, VersionString: version.AgentName, Children: children(self)}
	if ip != nil {
		n.Address = ip.String()
	}
	for _, id := range ids {
		if !visited[id] {
			visited[id] = true
			child := nodes[id]
			child.Children = children(id)
			n.Children = append(n.Children, child)
		}
	}
	if upstream := ch.UpstreamAddr(); upstream != "" {
		host, portString, _ := net.SplitHostPort(upstream)
		port, _ := strconv.Atoi(portString)
		sid, _, _ := ch.UpstreamNodeInfo()
		id := ""
		if !sid.IsEmpty() {
			id = gnuIDString(sid)
		}
		return []relayTreeNode{{SessionID: id, Address: host, Port: port, IsReceiving: ch.IsReceiving(), Children: []relayTreeNode{n}}}, nil
	}
	return []relayTreeNode{n}, nil
}
