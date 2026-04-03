package jsonrpc

import (
	"encoding/json"
	"fmt"

	"github.com/titagaki/peercast-mi/internal/channel"
)

type connEntry struct {
	ConnectionID   int     `json:"connectionId"`
	Type           string  `json:"type"`
	Status         string  `json:"status"`
	SendRate       int64   `json:"sendRate"`
	RecvRate       int64   `json:"recvRate"`
	ProtocolName   string  `json:"protocolName"`
	RemoteEndPoint *string `json:"remoteEndPoint"`
}

func (s *Server) getChannelConnections(ch *channel.Channel) (interface{}, *rpcError) {
	sourceProto := "RTMP"
	sourceAddr := fmt.Sprintf("127.0.0.1:%d", s.cfg.RTMPPort)
	if upstream := ch.UpstreamAddr(); upstream != "" {
		sourceProto = "PCP"
		sourceAddr = upstream
	}
	sourceStatus := "Idle"
	if ch.HasData() {
		sourceStatus = "Receiving"
	}
	result := []connEntry{
		{
			ConnectionID:   -1,
			Type:           "source",
			Status:         sourceStatus,
			SendRate:       0,
			RecvRate:       0,
			ProtocolName:   sourceProto,
			RemoteEndPoint: &sourceAddr,
		},
	}

	for _, ci := range ch.Connections() {
		typ := "relay"
		proto := "PCP"
		if ci.Type == channel.OutputStreamHTTP {
			typ = "direct"
			proto = "HTTP"
		}
		addr := ci.RemoteAddr
		result = append(result, connEntry{
			ConnectionID:   ci.ID,
			Type:           typ,
			Status:         "Connected",
			SendRate:       ci.SendRate,
			RecvRate:       0,
			ProtocolName:   proto,
			RemoteEndPoint: &addr,
		})
	}
	return result, nil
}

func (s *Server) stopChannelConnection(params json.RawMessage) (interface{}, *rpcError) {
	var args []json.RawMessage
	if err := json.Unmarshal(params, &args); err != nil || len(args) < 2 {
		return nil, &rpcError{Code: errCodeInvalidParams, Message: "expected [channelId, connectionId]"}
	}
	var chanIDStr string
	if err := json.Unmarshal(args[0], &chanIDStr); err != nil {
		return nil, &rpcError{Code: errCodeInvalidParams, Message: "invalid channelId"}
	}
	ch, ok := s.lookupChannel(chanIDStr)
	if !ok {
		return nil, &rpcError{Code: errCodeInternal, Message: "channel not found"}
	}

	var connID int
	if err := json.Unmarshal(args[1], &connID); err != nil {
		return nil, &rpcError{Code: errCodeInvalidParams, Message: "invalid connectionId"}
	}

	// Only relay (PCP) connections can be stopped.
	for _, ci := range ch.Connections() {
		if ci.ID == connID {
			if ci.Type != channel.OutputStreamPCP {
				return false, nil
			}
			return ch.CloseConnection(connID), nil
		}
	}
	return false, nil
}
