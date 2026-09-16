package main

import (
	"context"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/titagaki/peercast-pcp/pcp"

	"github.com/titagaki/peercast-mi/internal/audit"
	"github.com/titagaki/peercast-mi/internal/channel"
	"github.com/titagaki/peercast-mi/internal/config"
	"github.com/titagaki/peercast-mi/internal/id"
	"github.com/titagaki/peercast-mi/internal/jsonrpc"
	"github.com/titagaki/peercast-mi/internal/relay"
	"github.com/titagaki/peercast-mi/internal/rtmp"
	"github.com/titagaki/peercast-mi/internal/servent"
	"github.com/titagaki/peercast-mi/internal/site"
	"github.com/titagaki/peercast-mi/internal/yp"
)

func main() {
	configPath := flag.String("config", "config.toml", "Path to config file")
	ypName := flag.String("yp", "", "YP name to use (default: first entry in config)")
	flag.Parse()

	// Minimal logger before config is loaded (errors only).
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.SlogLevel()})))

	// Create a context that is cancelled on SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	sessionID := id.NewRandom()
	broadcastID, err := id.LoadOrCreateBroadcastID(filepath.Join(filepath.Dir(*configPath), "broadcast_id"))
	if err != nil {
		slog.Error("broadcast ID: load failed", "err", err)
		os.Exit(1)
	}

	slog.Info("startup", "session_id", sessionID, "broadcast_id", broadcastID)

	mgr := channel.NewManager(broadcastID)
	var recorder *audit.Recorder
	if cfg.Audit.Enabled {
		if cfg.Audit.SpoolDir == "" {
			cfg.Audit.SpoolDir = filepath.Join(filepath.Dir(*configPath), "site-data", "audit")
		}
		db, openErr := audit.OpenMySQL()
		if openErr != nil {
			slog.Error("audit: database environment incomplete; events will remain in spool")
		} else {
			defer db.DB.Close()
		}
		recorder = audit.New(audit.Options{Node: cfg.Audit.NodeID, Dir: cfg.Audit.SpoolDir, Retention: time.Duration(cfg.Audit.RetentionDays) * 24 * time.Hour}, db)
		mgr.Audit = recorder
	}

	if cfg.PublicIPv4 != "" {
		mgr.Network.SetPublicIPv4(net.ParseIP(cfg.PublicIPv4))
		slog.Info("network: explicit public IPv4", "address", cfg.PublicIPv4)
	}
	mgr.MaxRelays, mgr.MaxListeners = cfg.MaxRelays, cfg.MaxListeners
	mgr.MaxRelaysTotal, mgr.MaxUpstreamKbps = cfg.MaxRelaysTotal, cfg.MaxUpstreamKbps
	mgr.ContentBufferSeconds = cfg.ContentBufferSeconds
	cachePath := filepath.Join(filepath.Dir(*configPath), "stream_keys.json")
	mgr.SetCachePath(cachePath)
	if err := mgr.LoadCache(); err != nil {
		if cfg.Site.Enabled {
			slog.Error("site: cannot load stream key cache", "err", err)
			os.Exit(1)
		}
		slog.Warn("stream key cache: load failed", "err", err)
	}
	// Relay clients are created by the manager on demand (see StartRelay);
	// inject the constructor here to keep channel independent of relay.
	mgr.NewRelay = func(ch *channel.Channel, upstreamAddr string) channel.RelayHandle {
		return relay.New(upstreamAddr, ch.ID, sessionID, uint16(cfg.PeercastPort), ch)
	}

	mgr.MaxRelayChannels = cfg.MaxRelayChannels

	// Start OutputListener.
	listener := servent.NewListener(sessionID, mgr, cfg.PeercastPort, cfg.MaxRelays, cfg.MaxRelaysTotal, cfg.MaxListeners, cfg.MaxUpstreamKbps)
	relayFromAny, err := cfg.RelayRequestFromAny()
	if err != nil {
		slog.Error("config: invalid", "err", err)
		os.Exit(1)
	}
	listener.RelayRequestFromAny = relayFromAny
	// Configure the website's HTTP viewing gate before accepting any traffic.
	var website *site.Server
	var siteHTTP *http.Server
	if cfg.Site.Enabled {
		// Keep history on the same persistent volume as configuration and keys.
		if cfg.Site.BroadcastHistoryDir == "" {
			cfg.Site.BroadcastHistoryDir = filepath.Join(filepath.Dir(*configPath), "site-data", "broadcast-history")
		}
		website, err = site.New(cfg.Site, mgr, cfg.PeercastPort, nil)
		if err != nil {
			slog.Error("site: invalid configuration", "err", err)
			os.Exit(1)
		}
		listener.ViewerToken = website.ViewerToken()
		if cfg.Site.DevLogin {
			slog.Warn("site: DEVELOPMENT LOGIN enabled; keep the site and Vite private", "origin", cfg.Site.Origin)
		}
	}
	if err := listener.Listen(); err != nil {
		slog.Error("output: listen failed", "err", err)
		os.Exit(1)
	}
	slog.Info("output: listening", "port", cfg.PeercastPort)
	go func() {
		if err := listener.Serve(); err != nil {
			slog.Error("output: listener stopped", "err", err)
		}
	}()

	// JSON-RPC API will be wired after YP setup (ypBumper may be nil).
	var ypBumper jsonrpc.YPBumper

	// Start YPClient if configured.
	ypEntry, err := cfg.FindYP(*ypName)
	if err != nil {
		slog.Info("yp: skipping", "reason", err)
	} else {
		hostPort, err := ypEntry.HostPort()
		if err != nil {
			slog.Error("yp: invalid addr", "addr", ypEntry.Addr, "err", err)
			os.Exit(1)
		}
		ypClient := yp.New(hostPort, sessionID, broadcastID, mgr, cfg.PeercastPort, cfg.MaxRelays, cfg.MaxListeners)
		ypClient.OnGlobalIP = func(ip uint32) {
			slog.Debug("global IP acquired", "ip", pcp.IPv4FromUint32(ip))
			listener.SetGlobalIP(ip)
			mgr.SetGlobalIP(ip)
		}
		ypBumper = ypClient
		go func() {
			slog.Info("yp: connecting", "addr", ypEntry.Addr, "name", ypEntry.Name)
			ypClient.Run()
		}()
		defer ypClient.Stop()
	}

	// Wire on-demand relay: auto-start relay when /pls/ or /stream/ is
	// requested for an unknown channel. Without a tip, every configured YP is
	// asked for the tracker in turn
	// (PeerCastStation 互換: PeerCast.RelayChannel → FindTracker)。
	var ypAddrs []string
	for _, y := range cfg.YPs {
		if hp, err := y.HostPort(); err == nil {
			ypAddrs = append(ypAddrs, hp)
		} else {
			slog.Warn("yp: invalid addr, not used for tracker lookup", "addr", y.Addr, "err", err)
		}
	}
	listener.OnDemandRelay = func(channelID pcp.GnuID, upstreamAddr string) (*channel.Channel, error) {
		if upstreamAddr == "" {
			if ch, ok := mgr.GetByID(channelID); ok {
				return ch, nil
			}
			var err error
			upstreamAddr, err = relay.FindTracker(context.Background(), ypAddrs, channelID, sessionID, mgr.GlobalIP())
			if err != nil {
				return nil, err
			}
		}
		return mgr.StartRelay(channelID, upstreamAddr)
	}

	// Wire JSON-RPC API handler into the listener.
	apiServer := jsonrpc.New(sessionID, mgr, cfg, ypBumper)
	listener.SetAPIHandler(apiServer.Handler())
	slog.Info("api: JSON-RPC ready", "port", cfg.PeercastPort)
	if website != nil {
		website.SetCatalog(apiServer.Catalog())
		if cfg.Audit.Enabled {
			readDB, err := audit.OpenMySQL()
			if err != nil {
				slog.Error("audit: history reader database environment incomplete")
			} else {
				defer readDB.DB.Close()
				website.SetAuditReader(readDB)
			}
		}
		website.SetAdminHandler(apiServer.Handler())
		website.SetBump(func() {
			if ypBumper != nil {
				ypBumper.Bump()
			}
		})
		ln, err := net.Listen("tcp", website.ListenAddress())
		if err != nil {
			slog.Error("site: listen failed", "err", err)
			os.Exit(1)
		}
		srv := &http.Server{Handler: website.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
		siteHTTP = srv
		go func() {
			if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
				slog.Error("site: server stopped", "err", err)
				stop()
			}
		}()
		defer srv.Close()
		slog.Info("site: listening", "address", website.ListenAddress())
	}

	// Start channel cleaner for idle relay channels.
	if cfg.ChannelCleanupMinutes > 0 {
		cleaner := channel.NewCleaner(mgr, time.Duration(cfg.ChannelCleanupMinutes)*time.Minute)
		go cleaner.Run()
		defer cleaner.Stop()
	}

	// Start RTMP server.
	rtmpServer := rtmp.NewServer(mgr, cfg.RTMPPort)
	if err := rtmpServer.Listen(); err != nil {
		slog.Error("rtmp: listen failed", "err", err)
		os.Exit(1)
	}
	slog.Info("rtmp: listening", "port", cfg.RTMPPort)
	go func() {
		if err := rtmpServer.Serve(); err != nil {
			slog.Error("rtmp: server stopped", "err", err)
		}
	}()

	// Wait for shutdown signal.
	<-ctx.Done()

	slog.Info("shutting down")
	if siteHTTP != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := siteHTTP.Shutdown(shutdownCtx); err != nil {
			siteHTTP.Close()
		}
		cancel()
	}
	rtmpServer.Close()
	listener.Close()
	mgr.StopAll()
	if recorder != nil {
		flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := recorder.Close(flushCtx); err != nil {
			slog.Error("audit: shutdown flush timed out")
		}
		cancel()
	}
}
