package jsonrpc

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"

	"github.com/titagaki/peercast-pcp/pcp"

	"github.com/titagaki/peercast-mi/internal/audit"
	"github.com/titagaki/peercast-mi/internal/catalog"
	"github.com/titagaki/peercast-mi/internal/channel"
	"github.com/titagaki/peercast-mi/internal/config"
)

// YPBumper is the subset of yp.Client that the JSON-RPC server needs.
type YPBumper interface {
	Bump()
}

// ChannelManager is the subset of channel.Manager that the JSON-RPC server needs.
type ChannelManager interface {
	IssueStreamKey(accountName, streamKey string) error
	RevokeStreamKey(accountName string) bool
	ListStreamKeys() []channel.StreamKeyEntry
	Broadcast(streamKey string, info channel.ChannelInfo, track channel.TrackInfo) (*channel.Channel, error)
	Stop(channelID pcp.GnuID) bool
	GetByID(channelID pcp.GnuID) (*channel.Channel, bool)
	StreamKeyByID(channelID pcp.GnuID) (string, bool)
	List() []*channel.Channel
}

// Server handles JSON-RPC 2.0 requests at POST /api/1.
type Server struct {
	actor     audit.Actor
	catalog   *catalog.Catalog
	sessionID pcp.GnuID
	mgr       ChannelManager
	cfg       *config.Config
	ypClient  YPBumper // may be nil
}

// New creates a new JSON-RPC Server.
func New(sessionID pcp.GnuID, mgr ChannelManager, cfg *config.Config, ypClient YPBumper) *Server {
	return &Server{
		catalog:   catalog.New(cfg.YPs),
		sessionID: sessionID,
		mgr:       mgr,
		cfg:       cfg,
		ypClient:  ypClient,
	}
}

// Catalog is shared with the unprivileged site; it contains no admin secrets.
func (s *Server) Catalog() *catalog.Catalog { return s.catalog }

// Handler returns an http.Handler for POST /api/1.
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Cross-origin requests from a browser carry an Origin header. Only
		// loopback origins and explicitly configured ones are allowed; anything
		// else is rejected outright so that an arbitrary web page cannot drive
		// the (unauthenticated) localhost API through the user's browser.
		if origin := r.Header.Get("Origin"); origin != "" {
			if !s.isAllowedOrigin(origin) {
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Add("Vary", "Origin")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if !isLocalhost(r.RemoteAddr) {
			if !s.checkBasicAuth(r) {
				w.Header().Set("WWW-Authenticate", `Basic realm="peercast-mi"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}

		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeRPCError(w, nil, errCodeParse, "parse error")
			return
		}

		slog.Debug("jsonrpc: request", "remote", r.RemoteAddr, "method", req.Method)
		requestServer := *s
		requestServer.actor = audit.Actor{Source: "admin", IP: audit.IP(r.RemoteAddr)}
		if !isLocalhost(r.RemoteAddr) {
			requestServer.actor.Account = "admin:basic"
		}
		if a, ok := audit.ContextActor(r.Context()); ok {
			requestServer.actor = a
		}
		result, rpcErr := requestServer.dispatch(req.Method, req.Params)
		if rpcErr != nil {
			kind := map[string]string{"broadcastChannel": "broadcast.create", "stopChannel": "broadcast.end", "setChannelInfo": "broadcast.metadata", "issueStreamKey": "key.issue", "revokeStreamKey": "key.revoke"}[req.Method]
			if kind != "" {
				requestServer.auditEvent(audit.Event{Type: kind, Outcome: "failure", Reason: "operation_rejected"})
			}
		}

		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      req.ID,
		}
		if rpcErr != nil {
			resp["error"] = rpcErr
		} else {
			resp["result"] = result
		}
		json.NewEncoder(w).Encode(resp)
	})
}

// isAllowedOrigin reports whether a CORS request from origin may use the API.
// Loopback origins (any port) are always allowed so the bundled Web UI works
// from a local dev server; other origins must be listed in cfg.AllowedOrigins.
func (s *Server) isAllowedOrigin(origin string) bool {
	for _, allowed := range s.cfg.AllowedOrigins {
		if allowed == origin {
			return true
		}
	}
	u, err := url.Parse(origin)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}
	host := u.Hostname()
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// checkBasicAuth returns true if the request carries valid Basic auth credentials
// as configured in cfg. Returns false if credentials are not configured or do not match.
func (s *Server) checkBasicAuth(r *http.Request) bool {
	if s.cfg.AdminUser == "" || s.cfg.AdminPass == "" {
		return false
	}
	user, pass, ok := r.BasicAuth()
	return ok && user == s.cfg.AdminUser && pass == s.cfg.AdminPass
}

// ---------------------------------------------------------------------------
// JSON-RPC wire types
// ---------------------------------------------------------------------------

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      json.RawMessage `json:"id"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

const (
	errCodeParse          = -32700
	errCodeMethodNotFound = -32601
	errCodeInvalidParams  = -32602
	errCodeInternal       = -32603
)

// isLocalhost reports whether remoteAddr (in "host:port" form) is a
// loopback address.
func isLocalhost(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func writeRPCError(w http.ResponseWriter, id json.RawMessage, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"jsonrpc": "2.0",
		"error":   &rpcError{Code: code, Message: message},
		"id":      id,
	})
}

// ---------------------------------------------------------------------------
// Dispatch
// ---------------------------------------------------------------------------

func (s *Server) dispatch(method string, params json.RawMessage) (interface{}, *rpcError) {
	switch method {
	case "getVersionInfo":
		return s.getVersionInfo()
	case "getSettings":
		return s.getSettings()
	case "issueStreamKey":
		return s.issueStreamKey(params)
	case "revokeStreamKey":
		return s.revokeStreamKey(params)
	case "listStreamKeys":
		return s.listStreamKeys()
	case "broadcastChannel":
		return s.broadcastChannel(params)
	case "getChannels":
		return s.getChannels()
	case "getChannelInfo":
		return s.withChannel(params, s.getChannelInfo)
	case "getChannelStatus":
		return s.withChannel(params, s.getChannelStatus)
	case "setChannelInfo":
		return s.setChannelInfo(params)
	case "stopChannel":
		return s.withChannel(params, s.stopChannel)
	case "bumpChannel":
		return s.bumpChannelWithParams(params)
	case "getChannelConnections":
		return s.withChannel(params, s.getChannelConnections)
	case "stopChannelConnection":
		return s.stopChannelConnection(params)
	case "getYellowPages":
		return s.getYellowPages()
	case "updateYPChannels":
		rows, _, err := s.catalog.Update(context.Background())
		if err != nil {
			return nil, &rpcError{Code: errCodeInternal, Message: "YP directory update failed"}
		}
		return rows, nil
	case "getChannelRelayTree":
		return s.withChannel(params, s.getChannelRelayTree)
	default:
		return nil, &rpcError{Code: errCodeMethodNotFound, Message: fmt.Sprintf("method not found: %s", method)}
	}
}

// withChannel parses the first positional param as a channel ID, looks up the
// active channel in the manager, then calls fn with it.
func (s *Server) withChannel(params json.RawMessage, fn func(*channel.Channel) (interface{}, *rpcError)) (interface{}, *rpcError) {
	var args []json.RawMessage
	if err := json.Unmarshal(params, &args); err != nil || len(args) == 0 {
		return nil, &rpcError{Code: errCodeInvalidParams, Message: "channelId required"}
	}
	var chanIDStr string
	if err := json.Unmarshal(args[0], &chanIDStr); err != nil {
		return nil, &rpcError{Code: errCodeInvalidParams, Message: "invalid channelId"}
	}
	ch, ok := s.lookupChannel(chanIDStr)
	if !ok {
		return nil, &rpcError{Code: errCodeInternal, Message: "channel not found"}
	}
	return fn(ch)
}

func (s *Server) lookupChannel(chanIDStr string) (*channel.Channel, bool) {
	b, err := hex.DecodeString(chanIDStr)
	if err != nil || len(b) != 16 {
		return nil, false
	}
	var id pcp.GnuID
	copy(id[:], b)
	return s.mgr.GetByID(id)
}

func gnuIDString(id pcp.GnuID) string {
	return hex.EncodeToString(id[:])
}

func (s *Server) auditEvent(e audit.Event) {
	e.Actor = s.actor
	if m, ok := s.mgr.(interface{ AuditSink() audit.Sink }); ok {
		audit.Send(m.AuditSink(), e)
	}
}
