package servent

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/titagaki/peercast-pcp/pcp"

	"github.com/titagaki/peercast-mi/internal/channel"
	"github.com/titagaki/peercast-mi/internal/version"
)

// ChannelStore provides channel lookup by ID and aggregate statistics.
type ChannelStore interface {
	GetByID(channelID pcp.GnuID) (*channel.Channel, bool)
	TotalRelays() int
	TotalSendRate() int64
}

// OnDemandRelayFunc is called by the /pls/ and /stream/ handlers when a
// channel is not found locally. upstreamAddr is the validated tip query
// parameter, or "" when the request had none; implementations then have to
// find the tracker themselves (e.g. by asking the YPs). Implementations
// should register the channel in the manager, start a relay client and
// return the channel. If the channel is already active, implementations
// should return it without starting anything.
type OnDemandRelayFunc func(channelID pcp.GnuID, upstreamAddr string) (*channel.Channel, error)

// Listener accepts incoming connections on the PeerCast port and dispatches them
// to the appropriate output stream handler.
type Listener struct {
	sessionID       pcp.GnuID
	mgr             ChannelStore
	port            int
	maxRelays       int           // 0 = unlimited (per-channel)
	maxRelaysTotal  int           // 0 = unlimited (global)
	maxListeners    int           // 0 = unlimited (per-channel)
	maxUpstreamKbps int           // 0 = unlimited (global, kbps)
	globalIP        atomic.Uint32 // learned from YP oleh
	listener        net.Listener
	nextConnID      atomic.Int64
	apiHandler      http.Handler      // JSON-RPC handler for POST /api/; may be nil
	OnDemandRelay   OnDemandRelayFunc // optional: auto-start relay on /pls/ and /stream/ requests
	// RelayRequestFromAny lets any remote start an on-demand relay. When
	// false (the default from config) only loopback and private addresses
	// may; other remotes get 403 for unknown channels, like
	// PeerCastStation's GlobalAccepts without Play and peercast-yt's
	// isPrivate() gate. Viewing registered channels is not affected.
	RelayRequestFromAny bool
	admitMu             sync.Mutex // serializes global limit check + TryAddOutput
}

// NewListener creates a new Listener.
// maxRelays and maxListeners set the per-channel connection limits (0 = unlimited).
// maxRelaysTotal and maxUpstreamKbps set global limits (0 = unlimited).
func NewListener(sessionID pcp.GnuID, mgr ChannelStore, port, maxRelays, maxRelaysTotal, maxListeners, maxUpstreamKbps int) *Listener {
	return &Listener{
		sessionID:       sessionID,
		mgr:             mgr,
		port:            port,
		maxRelays:       maxRelays,
		maxRelaysTotal:  maxRelaysTotal,
		maxListeners:    maxListeners,
		maxUpstreamKbps: maxUpstreamKbps,
	}
}

// SetAPIHandler sets the HTTP handler used for POST /api/ requests.
func (l *Listener) SetAPIHandler(h http.Handler) {
	l.apiHandler = h
}

// Listen binds to the configured PeerCast port. It must be called before Serve.
func (l *Listener) Listen() error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", l.port))
	if err != nil {
		return fmt.Errorf("servent: listen: %w", err)
	}
	l.listener = ln
	return nil
}

// Serve accepts incoming connections. Listen must be called first.
func (l *Listener) Serve() error {
	ln := l.listener
	defer ln.Close()
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go l.handle(conn)
	}
}

// SetGlobalIP updates the global IP address reported in PCPHost atoms.
// It is called with the IP learned from the YP oleh.
func (l *Listener) SetGlobalIP(ip uint32) {
	l.globalIP.Store(ip)
}

// Close shuts down the listener.
func (l *Listener) Close() {
	if l.listener != nil {
		l.listener.Close()
	}
}

func (l *Listener) handle(conn net.Conn) {
	cc := newCountingConn(conn)
	br := bufio.NewReader(conn)

	// Peek enough bytes to identify the protocol and extract a 32-hex channel ID.
	// "GET /channel/<32-hex>" = 13 + 32 = 45 chars; 64 bytes is sufficient.
	peek, err := br.Peek(64)
	if err != nil && len(peek) < 4 {
		conn.Close()
		return
	}

	switch {
	case bytes.HasPrefix(peek, []byte("GET /channel/")):
		l.handlePCPRelay(cc, br, peek)
	case bytes.HasPrefix(peek, []byte("GET /stream/")):
		l.handleHTTPStream(cc, br)
	case bytes.HasPrefix(peek, []byte("GET /pls/")):
		l.handlePLS(cc, br)
	case bytes.HasPrefix(peek, []byte("pcp\n")):
		slog.Debug("servent: ping", "remote", conn.RemoteAddr())
		handlePing(conn, br, l.sessionID)
	case bytes.HasPrefix(peek, []byte("POST /api")), bytes.HasPrefix(peek, []byte("OPTIONS /api")):
		if l.apiHandler != nil {
			l.handleAPIRequest(conn, br)
		} else {
			conn.Close()
		}

	default:
		slog.Warn("servent: unknown protocol", "remote", conn.RemoteAddr(), "peek", string(peek))
		conn.Close()
	}
}

func (l *Listener) handlePCPRelay(cc *countingConn, br *bufio.Reader, peek []byte) {
	channelID, ok := parseChannelIDFromPath(peek, "/channel/")
	if !ok {
		slog.Warn("pcp: bad channel path", "remote", cc.RemoteAddr())
		cc.Close()
		return
	}
	ch, ok := l.mgr.GetByID(channelID)
	if !ok {
		slog.Info("pcp: channel not found", "remote", cc.RemoteAddr(), "id", hex.EncodeToString(channelID[:]))
		io.WriteString(cc, statusNotFound)
		cc.Close()
		return
	}
	// PeerCastStation 互換: チャンネルがデータ受信中でなければ 404 を返す。
	if !ch.HasData() {
		slog.Info("pcp: channel not receiving", "remote", cc.RemoteAddr(), "id", hex.EncodeToString(channelID[:]))
		io.WriteString(cc, statusNotFound)
		cc.Close()
		return
	}
	id := int(l.nextConnID.Add(1))
	h := newPCPOutputStream(cc, br, l.sessionID, ch, id, l.globalIP.Load(), uint16(l.port), l.maxRelays, l.maxListeners)

	// Check admission before handshake to determine HTTP status code (200 vs 503).
	admitted := l.canAdmitRelay(ch, cc.RemoteAddr())
	startPos, err := h.handshake(admitted)
	if err != nil {
		slog.Error("pcp: handshake error", "remote", cc.RemoteAddr(), "id", id, "err", err)
		cc.Close()
		return
	}
	if !admitted {
		slog.Info("pcp: rejected (relay full)", "remote", cc.RemoteAddr(), "id", id)
		h.sendRelayDenied()
		cc.Close()
		return
	}

	// Atomically add to channel (may fail if slot was taken during handshake).
	if !l.tryAdmit(ch, h) {
		slog.Info("pcp: rejected (relay full)", "remote", cc.RemoteAddr(), "id", id)
		h.sendRelayDenied()
		cc.Close()
		return
	}
	slog.Info("pcp: relay connected", "remote", cc.RemoteAddr(), "id", id)
	h.runStreaming(startPos)
	ch.RemoveOutput(h)
}

// HTTP status lines written by the /pls/ and /stream/ handlers before any
// response body has been started.
const (
	statusBadRequest         = "HTTP/1.0 400 Bad Request\r\n\r\n"
	statusForbidden          = "HTTP/1.0 403 Forbidden\r\n\r\n"
	statusNotFound           = "HTTP/1.0 404 Not Found\r\n\r\n"
	statusServiceUnavailable = "HTTP/1.0 503 Service Unavailable\r\n\r\n"
	statusGatewayTimeout     = "HTTP/1.0 504 Gateway Timeout\r\n\r\n"
)

// viewerRequest is the part of a /pls/ or /stream/ request the handlers act
// on. It is parsed once by parseViewerRequest so that a handler never reads
// the request a second time.
type viewerRequest struct {
	channelID pcp.GnuID
	tip       string // validated "host:port", or "" when absent
	host      string // Host header, or "" when absent
}

// parseViewerRequest reads one HTTP request and extracts the channel ID from
// the path (prefix + 32 hex digits, optionally followed by an extension such
// as ".flv") and the tip query parameter. It returns the HTTP status line to
// send when the request is malformed.
func parseViewerRequest(br *bufio.Reader, prefix string) (viewerRequest, string, error) {
	req, err := http.ReadRequest(br)
	if err != nil {
		return viewerRequest{}, "", err
	}
	req.Body.Close()

	channelID, ok := parseChannelIDPath(req.URL.Path, prefix)
	if !ok {
		return viewerRequest{}, statusBadRequest, nil
	}
	tip, ok := parseTip(req.URL.Query().Get("tip"))
	if !ok {
		return viewerRequest{}, statusBadRequest, nil
	}
	return viewerRequest{channelID: channelID, tip: tip, host: req.Host}, "", nil
}

// parseChannelIDPath extracts the channel ID from a URL path of the form
// prefix + 32 hex digits, optionally followed by "." and an extension
// (PeerCastStation 互換: "/stream/<id>.flv" のような拡張子付きを許す)。
func parseChannelIDPath(path, prefix string) (pcp.GnuID, bool) {
	if !strings.HasPrefix(path, prefix) {
		return pcp.GnuID{}, false
	}
	rest := path[len(prefix):]
	if len(rest) < 32 {
		return pcp.GnuID{}, false
	}
	if ext := rest[32:]; ext != "" {
		if ext[0] != '.' || len(ext) == 1 || strings.ContainsAny(ext[1:], "/.") {
			return pcp.GnuID{}, false
		}
	}
	b, err := hex.DecodeString(rest[:32])
	if err != nil || len(b) != 16 {
		return pcp.GnuID{}, false
	}
	var id pcp.GnuID
	copy(id[:], b)
	return id, true
}

// parseTip validates a tip query parameter. An empty tip is allowed and
// returned as ""; otherwise the value must be "host:port" with a non-empty
// host and a port in 1..65535. The tip is external input that ends up in a
// net.Dial, so nothing else is accepted.
func parseTip(tip string) (string, bool) {
	if tip == "" {
		return "", true
	}
	host, portStr, err := net.SplitHostPort(tip)
	if err != nil || host == "" {
		return "", false
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return "", false
	}
	return tip, true
}

// lookupChannel returns the channel for channelID, starting an on-demand
// relay through OnDemandRelay when it is not registered. Creation and
// registration happen once inside OnDemandRelay (Manager.StartRelay), so
// concurrent requests for the same channel share one relay. When no channel
// results, the HTTP status line to answer with is returned.
func (l *Listener) lookupChannel(channelID pcp.GnuID, tip string, remote net.Addr) (*channel.Channel, string) {
	if ch, ok := l.mgr.GetByID(channelID); ok {
		return ch, ""
	}
	if l.OnDemandRelay == nil {
		return nil, statusNotFound
	}
	if !l.RelayRequestFromAny && !isPrivateAddr(remote) {
		slog.Info("servent: relay request refused (remote not private)", "remote", remote, "id", hex.EncodeToString(channelID[:]))
		return nil, statusForbidden
	}
	ch, err := l.OnDemandRelay(channelID, tip)
	if err != nil {
		slog.Warn("servent: auto-relay failed", "remote", remote, "id", hex.EncodeToString(channelID[:]), "tip", tip, "err", err)
		if errors.Is(err, channel.ErrRelayChannelLimit) {
			return nil, statusServiceUnavailable
		}
		return nil, statusNotFound
	}
	if ch == nil {
		return nil, statusNotFound
	}
	return ch, ""
}

// isPrivateAddr reports whether remote is a loopback, private (RFC 1918 /
// fc00::/7) or link-local address. Non-IP addresses are not private.
func isPrivateAddr(remote net.Addr) bool {
	tcp, ok := remote.(*net.TCPAddr)
	if !ok {
		return false
	}
	ip := tcp.IP
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

func (l *Listener) handlePLS(cc *countingConn, br *bufio.Reader) {
	defer cc.Close()

	vr, status, err := parseViewerRequest(br, "/pls/")
	if err != nil {
		return
	}
	if status != "" {
		io.WriteString(cc, status)
		return
	}

	ch, status := l.lookupChannel(vr.channelID, vr.tip, cc.RemoteAddr())
	if status != "" {
		slog.Info("pls: channel unavailable", "remote", cc.RemoteAddr(), "id", hex.EncodeToString(vr.channelID[:]), "status", strings.TrimSpace(status))
		io.WriteString(cc, status)
		return
	}

	// クライアントがアクセスに使ったホスト名/ポートをそのまま流用する。
	// localhost 固定だと LAN 越し視聴や WSL mirrored 環境で繋がらないため。
	host := vr.host
	if host == "" {
		host = fmt.Sprintf("localhost:%d", l.port)
	}
	streamURL := fmt.Sprintf("http://%s/stream/%s", host, hex.EncodeToString(vr.channelID[:]))
	name := ch.Info().Name
	if name == "" {
		name = hex.EncodeToString(vr.channelID[:])
	}
	body := fmt.Sprintf("#EXTM3U\n#EXTINF:-1,%s\n%s\n", name, streamURL)

	var sb strings.Builder
	sb.WriteString("HTTP/1.0 200 OK\r\n")
	sb.WriteString("Content-Type: audio/x-mpegurl\r\n")
	sb.WriteString(fmt.Sprintf("Content-Length: %d\r\n", len(body)))
	sb.WriteString("\r\n")
	sb.WriteString(body)
	io.WriteString(cc, sb.String())
	slog.Info("pls: sent playlist", "remote", cc.RemoteAddr(), "channel", name)
}

func (l *Listener) handleHTTPStream(cc *countingConn, br *bufio.Reader) {
	vr, status, err := parseViewerRequest(br, "/stream/")
	if err != nil {
		slog.Debug("http: read request error", "remote", cc.RemoteAddr(), "err", err)
		cc.Close()
		return
	}
	if status != "" {
		slog.Warn("http: bad stream request", "remote", cc.RemoteAddr())
		io.WriteString(cc, status)
		cc.Close()
		return
	}
	ch, status := l.lookupChannel(vr.channelID, vr.tip, cc.RemoteAddr())
	if status != "" {
		slog.Info("http: channel unavailable", "remote", cc.RemoteAddr(), "id", hex.EncodeToString(vr.channelID[:]), "status", strings.TrimSpace(status))
		io.WriteString(cc, status)
		cc.Close()
		return
	}
	id := int(l.nextConnID.Add(1))
	h := newHTTPOutputStream(cc, ch, id)
	if !l.tryAdmit(ch, h) {
		io.WriteString(cc, statusServiceUnavailable)
		cc.Close()
		return
	}
	slog.Info("http: viewer connected", "remote", cc.RemoteAddr(), "id", id)
	h.run()
	ch.RemoveOutput(h)
}

// canAdmitRelay checks whether a new PCP relay from remote can be accepted,
// attempting to evict a firewalled downstream node if per-channel relay slots
// are full. A remote IP that was itself evicted recently is refused outright
// (PeerCastStation 互換: MakeRelayable(key) は BAN 中なら false)。
// Used to determine the HTTP status code (200 vs 503) before the PCP handshake.
func (l *Listener) canAdmitRelay(ch *channel.Channel, remote net.Addr) bool {
	l.admitMu.Lock()
	defer l.admitMu.Unlock()
	if l.maxRelaysTotal > 0 && l.mgr.TotalRelays() >= l.maxRelaysTotal {
		return false
	}
	if l.isUpstreamFull() {
		return false
	}
	if tcp, ok := remote.(*net.TCPAddr); ok && ch.HasBanned(tcp.IP.String()) {
		slog.Info("pcp: rejected (banned)", "remote", remote)
		return false
	}
	// Try to evict a firewalled relay if per-channel limit is reached.
	if !ch.MakeRelayable(l.maxRelays) {
		return false
	}
	return true
}

// tryAdmit atomically checks global limits and adds the output stream to the
// channel. Returns false if any limit is exceeded; the caller must close the
// connection. The mutex serializes the global-limit check and TryAddOutput so
// that concurrent connections cannot both pass the check before either is added.
func (l *Listener) tryAdmit(ch *channel.Channel, o channel.OutputStream) bool {
	l.admitMu.Lock()
	defer l.admitMu.Unlock()

	if o.Type() == channel.OutputStreamPCP && l.maxRelaysTotal > 0 && l.mgr.TotalRelays() >= l.maxRelaysTotal {
		slog.Info("servent: rejected (total relay full)", "remote", o.RemoteAddr())
		return false
	}
	if l.isUpstreamFull() {
		slog.Info("servent: rejected (upstream bandwidth full)", "remote", o.RemoteAddr())
		return false
	}
	if !ch.TryAddOutput(o, l.maxRelays, l.maxListeners) {
		slog.Info("servent: rejected (per-channel full)", "remote", o.RemoteAddr())
		return false
	}
	return true
}

// isUpstreamFull reports whether the total upstream bandwidth exceeds the limit.
func (l *Listener) isUpstreamFull() bool {
	if l.maxUpstreamKbps <= 0 {
		return false
	}
	// SendRate is bytes/sec; convert to kbps (kilobits per second).
	currentKbps := l.mgr.TotalSendRate() * 8 / 1000
	return currentKbps >= int64(l.maxUpstreamKbps)
}

// parseChannelIDFromPath extracts a 32-hex-char channel ID from the peeked
// bytes. pathPrefix is the URL path segment before the ID (e.g. "/channel/").
// The peek slice starts with "GET ".
func parseChannelIDFromPath(peek []byte, pathPrefix string) (pcp.GnuID, bool) {
	s := string(peek)
	idx := strings.Index(s, pathPrefix)
	if idx < 0 {
		return pcp.GnuID{}, false
	}
	start := idx + len(pathPrefix)
	if start+32 > len(s) {
		return pcp.GnuID{}, false
	}
	hexStr := s[start : start+32]
	b, err := hex.DecodeString(hexStr)
	if err != nil || len(b) != 16 {
		return pcp.GnuID{}, false
	}
	var id pcp.GnuID
	copy(id[:], b)
	return id, true
}

// handleAPIRequest handles a JSON-RPC request forwarded from the listener.
func (l *Listener) handleAPIRequest(conn net.Conn, br *bufio.Reader) {
	defer conn.Close()
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	defer req.Body.Close()
	req.RemoteAddr = conn.RemoteAddr().String()
	rw := newAPIResponseWriter(conn)
	l.apiHandler.ServeHTTP(rw, req)
	rw.flush()
}

// apiResponseWriter is a minimal http.ResponseWriter that buffers the response
// body and writes it to the underlying connection as a plain HTTP/1.0 response.
type apiResponseWriter struct {
	conn   net.Conn
	header http.Header
	status int
	body   bytes.Buffer
}

func newAPIResponseWriter(conn net.Conn) *apiResponseWriter {
	return &apiResponseWriter{conn: conn, header: make(http.Header), status: http.StatusOK}
}

func (w *apiResponseWriter) Header() http.Header         { return w.header }
func (w *apiResponseWriter) WriteHeader(code int)        { w.status = code }
func (w *apiResponseWriter) Write(b []byte) (int, error) { return w.body.Write(b) }

func (w *apiResponseWriter) flush() {
	body := w.body.Bytes()
	resp := &http.Response{
		StatusCode:    w.status,
		ProtoMajor:    1,
		ProtoMinor:    0,
		Header:        w.header,
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
	}
	resp.Write(w.conn)
}

// handlePing handles a firewall reachability check connection from the YP.
// The YP sends "pcp\n" + helo; we reply with oleh (sid) + quit.
func handlePing(conn net.Conn, br *bufio.Reader, sessionID pcp.GnuID) {
	defer conn.Close()

	// Read "pcp\n" magic: 4-byte tag + 4-byte length + 4-byte version payload.
	magic := make([]byte, 12)
	if _, err := io.ReadFull(br, magic); err != nil {
		return
	}

	heloAtom, err := pcp.ReadAtom(br)
	if err != nil || heloAtom.Tag != pcp.PCPHelo {
		return
	}

	oleh := (&pcp.HeloPacket{
		Agent:     version.AgentName,
		SessionID: sessionID,
		Version:   version.PCPVersion,
	}).BuildOlehAtom()
	if err := oleh.Write(conn); err != nil {
		return
	}

	pcp.NewIntAtom(pcp.PCPQuit, pcp.PCPErrorQuit+pcp.PCPErrorShutdown).Write(conn)
}
