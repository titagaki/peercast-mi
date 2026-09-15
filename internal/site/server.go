// Package site provides opt-in viewing and broadcasting with X authentication.
// Administrative JSON-RPC access requires an explicitly allowed X user.
package site

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/titagaki/peercast-mi/internal/catalog"
	"github.com/titagaki/peercast-mi/internal/channel"
	"github.com/titagaki/peercast-mi/internal/config"
	"github.com/titagaki/peercast-pcp/pcp"
)

const sessionCookie = "mi_session"
const flowCookie = "mi_oauth"
const sessionTTL = 12 * time.Hour

type user struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type session struct {
	User    user
	CSRF    string
	Expires time.Time
	Done    chan struct{}
}
type flow struct {
	State, Verifier string
	Expires         time.Time
	Next            string
}

type Server struct {
	trustedProxies                      []netip.Prefix
	boards                              *boardReader
	catalog                             *catalog.Catalog
	cfg                                 config.Site
	mgr                                 *channel.Manager
	bump                                func()
	clientID, clientSecret, viewerToken string
	adminIDs                            map[string]bool
	adminProxy                          *httputil.ReverseProxy
	client                              *http.Client
	tokenURL, meURL                     string
	proxy                               *httputil.ReverseProxy
	mu                                  sync.Mutex
	sessions                            map[string]*session
	flows                               map[string]flow
	viewers                             map[string]int
	total                               int
	// Serialize website mutations across all sessions, including key rotation.
	mutate sync.Mutex
}

func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func New(cfg config.Site, mgr *channel.Manager, backendPort int, bump func()) (*Server, error) {
	if cfg.MaxRelayChannels < 0 {
		return nil, errors.New("site.max_relay_channels must not be negative")
	}
	if cfg.MaxRelayChannels == 0 {
		cfg.MaxRelayChannels = 8
	}
	if cfg.BasePath != "" && !regexp.MustCompile(`^(/[A-Za-z0-9_-]+)+$`).MatchString(cfg.BasePath) {
		return nil, errors.New("site.base_path must be empty or slash-prefixed segments of letters, digits, underscores or hyphens without a trailing slash")
	}
	u, err := url.Parse(cfg.Origin)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("site.origin must be an absolute origin without a path")
	}
	loopback := u.Hostname() == "localhost"
	if ip := net.ParseIP(u.Hostname()); ip != nil {
		loopback = ip.IsLoopback()
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return nil, errors.New("site.origin requires HTTPS (HTTP is allowed only on loopback for development)")
	}
	cfg.Origin = strings.TrimSuffix(cfg.Origin, "/")
	if cfg.Listen == "" {
		cfg.Listen = "127.0.0.1:8080"
	}
	if cfg.DevLogin {
		host, _, err := net.SplitHostPort(cfg.Listen)
		ip := net.ParseIP(host)
		if err != nil || ip == nil || !ip.IsLoopback() || !loopback || u.Scheme != "http" {
			return nil, errors.New("site.dev_login requires an HTTP loopback origin and a literal loopback listen address; never expose development login publicly")
		}
	}
	if cfg.BroadcastHistoryDir == "" {
		cfg.BroadcastHistoryDir = "site-data/broadcast-history"
	}
	if cfg.UIDir == "" {
		cfg.UIDir = "ui/dist"
	}
	if cfg.MaxViewers == 0 {
		cfg.MaxViewers = 20
	}
	if cfg.MaxViewersPerUser == 0 {
		cfg.MaxViewersPerUser = 2
	}
	if cfg.MaxViewers < 1 || cfg.MaxViewersPerUser < 1 {
		return nil, errors.New("site viewer limits must be positive")
	}
	if r, e := url.Parse(cfg.RTMPURL); e != nil || r.Host == "" || (r.Scheme != "rtmp" && r.Scheme != "rtmps") || r.User != nil || r.RawQuery != "" || r.Fragment != "" {
		return nil, errors.New("site.rtmp_url must be the public RTMP(S) application URL")
	}
	adminIDs := make(map[string]bool)
	for _, value := range cfg.AdminXIDs {
		if value == "" || len(value) > 32 || strings.Trim(value, "0123456789") != "" {
			return nil, errors.New("site.admin_x_ids must contain numeric X user IDs")
		}
		adminIDs[value] = true
	}
	clientID, secret := os.Getenv("PEERCAST_X_CLIENT_ID"), os.Getenv("PEERCAST_X_CLIENT_SECRET")
	if !cfg.DevLogin && (clientID == "" || secret == "") {
		return nil, errors.New("PEERCAST_X_CLIENT_ID and PEERCAST_X_CLIENT_SECRET are required")
	}
	var trustedProxies []netip.Prefix
	for _, value := range cfg.TrustedProxies {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return nil, fmt.Errorf("site.trusted_proxies: invalid CIDR %q", value)
		}
		trustedProxies = append(trustedProxies, prefix)
	}
	s := &Server{trustedProxies: trustedProxies, adminIDs: adminIDs, cfg: cfg, mgr: mgr, bump: bump, clientID: clientID, clientSecret: secret, viewerToken: randomToken(),
		client:   &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		boards:   newBoardReader(),
		tokenURL: "https://api.x.com/2/oauth2/token", meURL: "https://api.x.com/2/users/me",
		sessions: make(map[string]*session), flows: make(map[string]flow), viewers: make(map[string]int)}
	backend, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", backendPort))
	s.adminProxy = &httputil.ReverseProxy{
		Rewrite: func(p *httputil.ProxyRequest) {
			p.SetURL(backend)
			p.Out.URL.Path = "/api/1"
			p.Out.URL.RawPath = ""
			p.Out.URL.RawQuery = ""
			// Only an authorized X administrator with valid CSRF reaches this proxy.
			// Never forward browser credentials or origin to the loopback-only hop.
			p.Out.Header = make(http.Header)
			p.Out.Header.Set("Content-Type", "application/json")
		},
		Transport: &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext, ResponseHeaderTimeout: 20 * time.Second},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, "管理APIに接続できません。", http.StatusBadGateway)
		},
	}

	s.proxy = &httputil.ReverseProxy{Rewrite: func(p *httputil.ProxyRequest) {
		p.SetURL(backend)
		p.Out.URL.Path = "/stream/" + p.In.PathValue("id")
		p.Out.URL.RawPath = ""
		p.Out.URL.RawQuery = ""
		p.Out.Header = make(http.Header)
		p.Out.Header.Set("Authorization", "Bearer "+s.viewerToken)
	}, FlushInterval: -1, Transport: &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext, ResponseHeaderTimeout: 15 * time.Second},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, "映像に接続できません。再試行してください。", http.StatusBadGateway)
		}}
	return s, nil
}

func (s *Server) ViewerToken() string   { return s.viewerToken }
func (s *Server) ListenAddress() string { return s.cfg.Listen }

// SetBump must be called before serving requests.
func (s *Server) SetBump(bump func()) { s.bump = bump }

// SetCatalog must be called before serving requests.
func (s *Server) SetCatalog(c *catalog.Catalog) { s.catalog = c }

func (s *Server) cookie(w http.ResponseWriter, name, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: s.cfg.BasePath + "/", MaxAge: maxAge, HttpOnly: true, Secure: strings.HasPrefix(s.cfg.Origin, "https://"), SameSite: http.SameSiteLaxMode})
}
func reply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// pruneLocked bounds memory and actively closes streams belonging to expired sessions.
func (s *Server) pruneLocked() {
	now := time.Now()
	for id, f := range s.flows {
		if !now.Before(f.Expires) {
			delete(s.flows, id)
		}
	}
	for id, ss := range s.sessions {
		if !now.Before(ss.Expires) {
			close(ss.Done)
			delete(s.sessions, id)
		}
	}
}
func (s *Server) authenticate(r *http.Request) *session {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked()
	return s.sessions[c.Value]
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	handle := func(pattern string, h http.Handler) {
		method, path, _ := strings.Cut(pattern, " ")
		mux.Handle(method+" "+s.cfg.BasePath+path, h)
	}
	handleFunc := func(pattern string, h http.HandlerFunc) { handle(pattern, h) }
	if s.cfg.BasePath != "" {
		mux.HandleFunc("GET "+s.cfg.BasePath, func(w http.ResponseWriter, r *http.Request) {
			target := s.cfg.BasePath + "/"
			if r.URL.RawQuery != "" {
				target += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, http.StatusPermanentRedirect)
		})
	}
	if s.cfg.DevLogin {
		handleFunc("POST /site/api/dev-login", s.developmentLogin)
	} else {
		handleFunc("GET /auth/x/start", s.login)
		handleFunc("GET /auth/x/callback", s.callback)
	}
	handleFunc("GET /site/api/me", func(w http.ResponseWriter, r *http.Request) {
		ss := s.authenticate(r)
		if ss == nil {
			reply(w, map[string]any{"user": nil, "devLogin": s.cfg.DevLogin})
			return
		}
		reply(w, map[string]any{"user": ss.User, "csrf": ss.CSRF, "admin": s.isAdmin(ss), "devLogin": s.cfg.DevLogin})
	})
	protected := func(pattern string, h func(http.ResponseWriter, *http.Request, *session)) {
		handleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			ss := s.authenticate(r)
			if ss == nil {
				http.Error(w, "X ログインが必要です。", 401)
				return
			}
			if r.Method != http.MethodGet && (r.Header.Get("Origin") != s.cfg.Origin || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(ss.CSRF)) != 1) {
				http.Error(w, "操作を確認できません。画面を更新してください。", 403)
				return
			}
			h(w, r, ss)
		})
	}
	protected("POST /site/api/logout", s.logout)
	protected("GET /site/api/channels", s.channels)
	protected("GET /site/api/directory", s.directory)
	protected("GET /site/api/channels/{id}/comments", s.comments)
	protected("GET /site/api/broadcast", s.broadcastInfo)
	protected("GET /site/api/broadcast/board", s.broadcastBoard)
	protected("POST /site/api/key", s.rotateKey)
	protected("POST /site/api/broadcast", s.broadcast)
	protected("DELETE /site/api/broadcast", s.stopBroadcast)
	protected("GET /site/stream/{id}", s.stream)
	// Serve only compiled UI assets, never project files.
	handleFunc("GET /favicon.svg", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(s.cfg.UIDir, "favicon.svg"))
	})
	handle("GET /assets/", http.StripPrefix(s.cfg.BasePath, http.FileServer(http.Dir(s.cfg.UIDir))))
	page := func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(s.cfg.UIDir, "index.html"))
	}
	handleFunc("GET /{$}", page)
	handleFunc("GET /channels/{id}", func(w http.ResponseWriter, r *http.Request) {
		if _, err := parseID(r.PathValue("id")); err != nil {
			http.NotFound(w, r)
			return
		}
		page(w, r)
	})
	handleFunc("GET /broadcast", page)
	// The UI gate renders login/permission states; the API independently checks authorization.
	handleFunc("GET /admin", page)
	handleFunc("GET /admin/{$}", page)
	protected("POST /admin/api/1", s.adminRPC)
	handleFunc("GET /watch", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, s.cfg.BasePath+"/", http.StatusFound)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; media-src 'self' blob:; connect-src 'self'; worker-src 'self' blob:; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		if s.cfg.DevLogin {
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			ip := net.ParseIP(host)
			origin, _ := url.Parse(s.cfg.Origin)
			if err != nil || ip == nil || !ip.IsLoopback() || r.Host != origin.Host {
				http.Error(w, "開発用サイトは設定された loopback ホストからのみ利用できます。", 403)
				return
			}
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != s.cfg.Origin {
			http.Error(w, "origin not allowed", 403)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.pruneLocked()
	if len(s.flows) >= 1000 {
		s.mu.Unlock()
		http.Error(w, "ログインが混み合っています。後で再試行してください。", 503)
		return
	}
	if c, e := r.Cookie(flowCookie); e == nil {
		delete(s.flows, c.Value)
	}
	id := randomToken()
	f := flow{randomToken(), randomToken(), time.Now().Add(5 * time.Minute), s.returnPath(r.URL.Query().Get("next"))}
	s.flows[id] = f
	s.mu.Unlock()
	s.cookie(w, flowCookie, id, 300)
	hash := sha256.Sum256([]byte(f.Verifier))
	q := url.Values{"response_type": {"code"}, "client_id": {s.clientID}, "redirect_uri": {s.cfg.Origin + s.cfg.BasePath + "/auth/x/callback"}, "scope": {"tweet.read users.read"}, "state": {f.State}, "code_challenge": {base64.RawURLEncoding.EncodeToString(hash[:])}, "code_challenge_method": {"S256"}}
	http.Redirect(w, r, "https://x.com/i/oauth2/authorize?"+q.Encode(), http.StatusFound)
}

func (s *Server) callback(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(flowCookie)
	if err != nil {
		http.Error(w, "ログインを最初からやり直してください。", 400)
		return
	}
	s.mu.Lock()
	f, ok := s.flows[c.Value]
	delete(s.flows, c.Value)
	s.mu.Unlock()
	s.cookie(w, flowCookie, "", -1)
	if !ok || !time.Now().Before(f.Expires) || subtle.ConstantTimeCompare([]byte(f.State), []byte(r.URL.Query().Get("state"))) != 1 || r.URL.Query().Get("code") == "" || r.URL.Query().Get("error") != "" {
		http.Error(w, "ログインが中止されたか、期限が切れました。", 400)
		return
	}
	u, err := s.exchange(r.Context(), r.URL.Query().Get("code"), f.Verifier)
	if err != nil {
		http.Error(w, "X ログインに失敗しました。サイトへ戻って再試行してください。", 502)
		return
	}
	if s.startSession(w, r, u) {
		http.Redirect(w, r, s.returnPath(f.Next), http.StatusSeeOther)
	}
}

// Development login still creates an ordinary session: ownership, CSRF,
// viewer limits and the media gate are not bypassed.
func (s *Server) developmentLogin(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Origin") != s.cfg.Origin {
		http.Error(w, "開発用ログインはサイト画面のボタンから行ってください。", 403)
		return
	}
	if s.startSession(w, r, user{ID: "dev-local", Name: "ローカル開発ユーザー"}) {
		reply(w, map[string]bool{"ok": true})
	}
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, u user) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked()
	if len(s.sessions) >= 10000 {
		http.Error(w, "ログインが混み合っています。", 503)
		return false
	}
	if old, e := r.Cookie(sessionCookie); e == nil {
		if ss := s.sessions[old.Value]; ss != nil {
			close(ss.Done)
			delete(s.sessions, old.Value)
		}
	}
	id := randomToken()
	s.sessions[id] = &session{u, randomToken(), time.Now().Add(sessionTTL), make(chan struct{})}
	s.cookie(w, sessionCookie, id, int(sessionTTL.Seconds()))
	return true
}

func (s *Server) exchange(ctx context.Context, code, verifier string) (user, error) {
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {s.cfg.Origin + s.cfg.BasePath + "/auth/x/callback"}, "code_verifier": {verifier}}
	req, err := http.NewRequestWithContext(ctx, "POST", s.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return user{}, err
	}
	req.SetBasicAuth(url.QueryEscape(s.clientID), url.QueryEscape(s.clientSecret))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var token struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	if err := s.requestJSON(req, &token); err != nil {
		return user{}, err
	}
	if token.AccessToken == "" || !strings.EqualFold(token.TokenType, "bearer") {
		return user{}, errors.New("invalid token")
	}
	req, err = http.NewRequestWithContext(ctx, "GET", s.meURL, nil)
	if err != nil {
		return user{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	var result struct {
		Data user `json:"data"`
	}
	if err := s.requestJSON(req, &result); err != nil {
		return user{}, err
	}
	if result.Data.ID == "" || len(result.Data.ID) > 32 || strings.Trim(result.Data.ID, "0123456789") != "" {
		return user{}, errors.New("invalid user")
	}
	return result.Data, nil // Provider access token is deliberately not stored.
}
func (s *Server) requestJSON(req *http.Request, v any) error {
	resp, err := s.client.Do(req)
	if err != nil {
		return errors.New("provider unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return errors.New("provider rejected request")
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(v)
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request, ss *session) {
	c, _ := r.Cookie(sessionCookie)
	s.mu.Lock()
	if current := s.sessions[c.Value]; current != nil {
		close(current.Done)
		delete(s.sessions, c.Value)
	}
	s.mu.Unlock()
	s.cookie(w, sessionCookie, "", -1)
	reply(w, map[string]bool{"ok": true})
}

func (s *Server) stream(w http.ResponseWriter, r *http.Request, ss *session) {
	id, err := parseID(r.PathValue("id"))
	if err != nil || r.URL.RawQuery != "" {
		http.Error(w, "invalid channel", 400)
		return
	}
	s.mu.Lock()
	if s.total >= s.cfg.MaxViewers || s.viewers[ss.User.ID] >= s.cfg.MaxViewersPerUser {
		s.mu.Unlock()
		w.Header().Set("Retry-After", "5")
		http.Error(w, "同時視聴数の上限です。別の再生を閉じてください。", 429)
		return
	}
	s.total++
	s.viewers[ss.User.ID]++
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.total--
		s.viewers[ss.User.ID]--
		if s.viewers[ss.User.ID] == 0 {
			delete(s.viewers, ss.User.ID)
		}
		s.mu.Unlock()
	}()
	ctx, cancel := context.WithDeadline(r.Context(), ss.Expires)
	defer cancel()
	go func() {
		select {
		case <-ss.Done:
			cancel()
		case <-ctx.Done():
		}
	}()
	if !s.ensureChannel(w, r.WithContext(ctx), id) {
		return
	}
	s.proxy.ServeHTTP(streamWriter{w}, r.WithContext(ctx))
}

// A stalled browser must not hold a viewer slot indefinitely. A server-wide
// WriteTimeout would instead terminate even healthy long-lived streams.
type streamWriter struct{ http.ResponseWriter }

func (w streamWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w streamWriter) Write(p []byte) (int, error) {
	_ = http.NewResponseController(w.ResponseWriter).SetWriteDeadline(time.Now().Add(15 * time.Second))
	return w.ResponseWriter.Write(p)
}
func parseID(value string) (pcp.GnuID, error) {
	var id pcp.GnuID
	b, e := hex.DecodeString(value)
	if e != nil || len(b) != len(id) {
		return id, errors.New("invalid ID")
	}
	copy(id[:], b)
	return id, nil
}

// returnPath accepts only public pages under this site's configured mount point.
func (s *Server) returnPath(value string) string {
	if s.cfg.BasePath != "" {
		if !strings.HasPrefix(value, s.cfg.BasePath+"/") {
			return s.cfg.BasePath + "/"
		}
		value = strings.TrimPrefix(value, s.cfg.BasePath)
	}
	if value == "/admin" || value == "/admin/" {
		return s.cfg.BasePath + "/admin"
	}
	return s.cfg.BasePath + safeReturnPath(value)
}
