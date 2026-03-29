package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/titagaki/peercast-mm/internal/channel"
	"github.com/titagaki/peercast-mm/internal/config"
	"github.com/titagaki/peercast-mm/internal/id"
	"github.com/titagaki/peercast-mm/internal/jsonrpc"
	"github.com/titagaki/peercast-mm/internal/rtmp"
	"github.com/titagaki/peercast-mm/internal/servent"
	"github.com/titagaki/peercast-mm/internal/yp"
)

func main() {
	configPath := flag.String("config", "config.toml", "Path to config file")
	ypName := flag.String("yp", "", "YP name to use (default: first entry in config)")
	chanName := flag.String("name", "", "Channel name (required)")
	chanGenre := flag.String("genre", "", "Channel genre")
	chanURL := flag.String("url", "", "Channel contact URL")
	chanDesc := flag.String("desc", "", "Channel description")
	chanBitrate := flag.Uint("bitrate", 0, "Channel bitrate (kbps)")
	flag.Parse()

	if *chanName == "" {
		log.Fatal("-name is required")
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	sessionID := id.NewRandom()
	broadcastID := id.NewRandom()
	channelID := id.ChannelID(broadcastID, *chanName, *chanGenre, uint32(*chanBitrate))

	log.Printf("SessionID:   %s", sessionID)
	log.Printf("BroadcastID: %s", broadcastID)
	log.Printf("ChannelID:   %s", channelID)

	ch := channel.New(channelID, broadcastID)
	ch.SetInfo(channel.ChannelInfo{
		Name:     *chanName,
		Genre:    *chanGenre,
		URL:      *chanURL,
		Desc:     *chanDesc,
		Bitrate:  uint32(*chanBitrate),
		Type:     "FLV",
		MIMEType: "video/x-flv",
		Ext:      ".flv",
	})

	// Start OutputListener.
	listener := servent.NewListener(sessionID, ch, cfg.PeercastPort)
	go func() {
		log.Printf("output: listening on :%d", cfg.PeercastPort)
		if err := listener.ListenAndServe(); err != nil {
			log.Printf("output: listener stopped: %v", err)
		}
	}()

	// JSON-RPC API will be wired after YP setup (ypClient may be nil).
	var ypClientForAPI *yp.Client

	// Start YPClient if configured.
	ypEntry, err := cfg.FindYP(*ypName)
	if err != nil {
		log.Printf("yp: %v (skipping)", err)
	} else {
		hostPort, err := ypEntry.HostPort()
		if err != nil {
			log.Fatalf("yp: invalid addr %q: %v", ypEntry.Addr, err)
		}
		ypClient := yp.New(hostPort, sessionID, broadcastID, ch)
		ypClientForAPI = ypClient
		go func() {
			log.Printf("yp: connecting to %s (%s)", ypEntry.Addr, ypEntry.Name)
			ypClient.Run()
		}()
		defer ypClient.Stop()
	}

	// Wire JSON-RPC API handler into the listener.
	apiServer := jsonrpc.New(sessionID, ch, cfg, ypClientForAPI)
	listener.SetAPIHandler(apiServer.Handler())
	log.Printf("api: JSON-RPC endpoint available at POST /api/1 on :%d", cfg.PeercastPort)

	// Start RTMP server.
	rtmpServer := rtmp.NewServer(ch, cfg.RTMPPort)
	go func() {
		log.Printf("rtmp: listening on :%d", cfg.RTMPPort)
		if err := rtmpServer.ListenAndServe(); err != nil {
			log.Printf("rtmp: server stopped: %v", err)
		}
	}()

	// Wait for interrupt.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	log.Println("shutting down...")
	rtmpServer.Close()
	listener.Close()
	ch.CloseAll()
}
