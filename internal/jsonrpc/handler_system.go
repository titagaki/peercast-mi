package jsonrpc

import "github.com/titagaki/peercast-mi/internal/version"

func (s *Server) getVersionInfo() (interface{}, *rpcError) {
	return map[string]string{"agentName": version.AgentName}, nil
}

func (s *Server) getSettings() (interface{}, *rpcError) {
	return map[string]int{
		"serverPort": s.cfg.PeercastPort,
		"rtmpPort":   s.cfg.RTMPPort,
	}, nil
}
