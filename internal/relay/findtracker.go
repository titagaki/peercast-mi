package relay

import (
	"bufio"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"time"

	"github.com/titagaki/peercast-pcp/pcp"

	"github.com/titagaki/peercast-mi/internal/version"
)

// findTrackerTimeout bounds one YP lookup (dial + handshake + host list).
const findTrackerTimeout = 10 * time.Second

// ErrTrackerNotFound is returned by FindTracker when no YP knows a tracker
// for the channel.
var ErrTrackerNotFound = errors.New("relay: tracker not found")

// FindTracker asks the YPs, in order, which node is the tracker of channelID
// and returns the first "host:port" found. It works the way
// PeerCastStation's PCPYellowPageClient.FindTracker does: a relay request
// for the channel is sent to the YP; a YP does not relay, so it answers
// HTTP 503 and hands out the hosts it knows for the channel (from the
// tracker's bcst), and the host flagged as tracker is taken. A 200 means the
// YP itself relays the channel and is used as the tracker.
//
// ourGlobalIP selects the tracker's local address when it is behind the same
// NAT as us (see selectSourceHost).
func FindTracker(ctx context.Context, ypAddrs []string, channelID, sessionID pcp.GnuID, ourGlobalIP uint32) (string, error) {
	for _, yp := range ypAddrs {
		addr, err := findTrackerAt(ctx, yp, channelID, sessionID, ourGlobalIP)
		if err != nil {
			slog.Info("relay: tracker lookup failed", "yp", yp, "channel", hex.EncodeToString(channelID[:]), "err", err)
			continue
		}
		slog.Info("relay: tracker found", "yp", yp, "channel", hex.EncodeToString(channelID[:]), "tracker", addr)
		return addr, nil
	}
	return "", ErrTrackerNotFound
}

func findTrackerAt(ctx context.Context, ypAddr string, channelID, sessionID pcp.GnuID, ourGlobalIP uint32) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, findTrackerTimeout)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", ypAddr)
	if err != nil {
		return "", fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}

	// Same request a relay would make; the helo is sent up front like
	// Client.handshake does (peercast-yt reads it after the 503 line).
	chanIDHex := hex.EncodeToString(channelID[:])
	req := fmt.Sprintf("GET /channel/%s HTTP/1.0\r\nHost: %s\r\nx-peercast-pcp: 1\r\n\r\n", chanIDHex, ypAddr)
	if _, err := io.WriteString(conn, req); err != nil {
		return "", fmt.Errorf("write GET: %w", err)
	}
	helo := (&pcp.HeloPacket{
		Agent:     version.AgentName,
		SessionID: sessionID,
		Version:   version.PCPVersion,
	}).BuildHeloAtom()
	if err := helo.Write(conn); err != nil {
		return "", fmt.Errorf("write helo: %w", err)
	}

	br := bufio.NewReader(conn)
	status, err := readHTTPStatus(br)
	if err != nil {
		return "", fmt.Errorf("read HTTP response: %w", err)
	}
	defer pcp.NewIntAtom(pcp.PCPQuit, pcp.PCPErrorQuit).Write(conn)

	switch status {
	case 200:
		// The YP relays the channel itself: treat it as the tracker
		// (PeerCastStation 互換)。
		return ypAddr, nil
	case 503:
		return readTrackerHost(br, channelID, ourGlobalIP)
	default:
		return "", fmt.Errorf("yp returned HTTP %d", status)
	}
}

// readTrackerHost reads atoms until quit and returns the address of the
// first host flagged as tracker for channelID.
func readTrackerHost(br *bufio.Reader, channelID pcp.GnuID, ourGlobalIP uint32) (string, error) {
	for {
		atom, err := pcp.ReadAtom(br)
		if err != nil {
			return "", fmt.Errorf("read atom: %w", err)
		}
		switch atom.Tag {
		case pcp.PCPHost:
			if cid := atom.FindChild(pcp.PCPHostChanID); cid != nil {
				if id, err := cid.GetID(); err == nil && id != channelID {
					continue
				}
			}
			node, ok := parseSourceNode(atom)
			if !ok || !node.IsTracker {
				continue
			}
			if addr := node.connectAddr(ourGlobalIP); addr != "" {
				return addr, nil
			}
		case pcp.PCPQuit:
			return "", ErrTrackerNotFound
		}
	}
}
