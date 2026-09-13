package servent

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/titagaki/peercast-mi/internal/channel"
)

const (
	directWriteTimeout = 60 * time.Second
)

// infoWaitTimeout bounds how long a viewer waits for the channel info
// (content type) before the response headers are sent; on timeout the
// viewer gets 504 (PeerCastStation 互換: WaitForReadyContentTypeAsync は 10 秒)。
// firstDataTimeout bounds how long a viewer then waits for the first packet
// after the response headers (e.g. while an on-demand relay is being
// established). Variables so tests can shorten them.
var (
	infoWaitTimeout  = 10 * time.Second
	firstDataTimeout = 30 * time.Second
)

// HTTPOutputStream sends raw FLV data to a media player over HTTP.
// The HTTP request has already been consumed by the Listener; run only
// writes.
type HTTPOutputStream struct {
	outputBase
	ch *channel.Channel
}

func newHTTPOutputStream(conn *countingConn, ch *channel.Channel, id int) *HTTPOutputStream {
	return &HTTPOutputStream{
		outputBase: newOutputBase(conn, id),
		ch:         ch,
	}
}

// Type implements channel.OutputStream.
func (o *HTTPOutputStream) Type() channel.OutputStreamType { return channel.OutputStreamHTTP }

func (o *HTTPOutputStream) run() {
	defer slog.Info("http: viewer disconnected", "remote", o.remoteAddr, "id", o.id)
	defer o.conn.Close()

	// Monitor the read side so we detect viewer disconnect even when no
	// new stream data is arriving (and thus no writes are attempted).
	go func() {
		buf := make([]byte, 1)
		for {
			if _, err := o.conn.Read(buf); err != nil {
				o.Close()
				return
			}
		}
	}()

	// Wait for the channel info before answering so that Content-Type and
	// the icy-* headers describe the stream: a freshly started relay has no
	// info until the upstream's chan atom arrives (that is before any data,
	// so this wait is short). Nothing has been written yet, so a timeout can
	// still be reported as an HTTP error.
	info := o.ch.Info()
	if info.Type == "" {
		timer := time.NewTimer(infoWaitTimeout)
		defer timer.Stop()
		for info.Type == "" {
			select {
			case <-o.infoCh:
				info = o.ch.Info()
			case <-o.closeCh:
				return
			case <-timer.C:
				slog.Info("http: no channel info within timeout", "remote", o.remoteAddr, "id", o.id)
				io.WriteString(o.conn, statusGatewayTimeout)
				return
			}
		}
	}

	// Send the response headers before waiting for data so the player does
	// not time out while the relay chain is being established.
	mimeType := info.MIMEType
	if mimeType == "" {
		mimeType = "video/x-flv"
	}
	var sb strings.Builder
	sb.WriteString("HTTP/1.0 200 OK\r\n")
	sb.WriteString(fmt.Sprintf("Content-Type: %s\r\n", sanitizeHeaderValue(mimeType)))
	sb.WriteString(fmt.Sprintf("icy-name: %s\r\n", sanitizeHeaderValue(info.Name)))
	sb.WriteString(fmt.Sprintf("icy-genre: %s\r\n", sanitizeHeaderValue(info.Genre)))
	sb.WriteString(fmt.Sprintf("icy-url: %s\r\n", sanitizeHeaderValue(info.URL)))
	sb.WriteString(fmt.Sprintf("icy-bitrate: %d\r\n", info.Bitrate))
	sb.WriteString("\r\n")
	if _, err := io.WriteString(o.conn, sb.String()); err != nil {
		return
	}

	if !o.ch.HasData() {
		// Wait for the first data packet (e.g. while relay is being established).
		select {
		case <-o.ch.Signal():
			// data arrived
		case <-o.closeCh:
			return
		case <-time.After(firstDataTimeout):
			slog.Info("http: no data within timeout", "remote", o.remoteAddr, "id", o.id)
			return
		}
	}

	// Send stream header (FLV header / codec config). A header notification
	// queued while waiting above (the relay's first header arrived after this
	// output was added) is covered by the read below; drop it so the header
	// is not sent twice.
	select {
	case <-o.headerCh:
	default:
	}
	header, _ := o.ch.Header()
	if len(header) > 0 {
		o.conn.SetWriteDeadline(time.Now().Add(directWriteTimeout))
		if _, err := o.conn.Write(header); err != nil {
			return
		}
	}

	// Stream data starting from the next keyframe. `sent` tracks the most
	// recently written packet by (Timestamp, Pos); this is used instead of a
	// raw position counter so that a header change (which rewinds the ring
	// buffer's position space) cannot cause stale-position filtering to drop
	// packets that are actually newer (PeerCastStation 互換).
	var sent channel.Content
	waitingForKeyframe := true

	// writeHeaderUpdate flushes the current stream header to the player and
	// resets the keyframe gating so the next-sent data starts a fresh GOP.
	// PeerCastStation 互換: ContentHeader 受信時に新ヘッダーを書き込んで継続。
	writeHeaderUpdate := func() error {
		h, _ := o.ch.Header()
		if len(h) == 0 {
			return nil
		}
		o.conn.SetWriteDeadline(time.Now().Add(directWriteTimeout))
		if _, err := o.conn.Write(h); err != nil {
			return err
		}
		sent = channel.Content{}
		waitingForKeyframe = true
		slog.Debug("http: header updated mid-stream", "remote", o.remoteAddr, "id", o.id)
		return nil
	}

	for {
		// Always service a pending header change before sending more data so
		// that a concurrent SetHeader+Write sequence cannot leak new body
		// bytes ahead of the new header.
		select {
		case <-o.headerCh:
			if err := writeHeaderUpdate(); err != nil {
				return
			}
		default:
		}

		sigCh := o.ch.Signal()
		packets := o.ch.PacketsAfter(sent)

		if len(packets) == 0 {
			select {
			case <-o.closeCh:
				return
			case <-o.headerCh:
				if err := writeHeaderUpdate(); err != nil {
					return
				}
				continue
			case <-sigCh:
				continue
			}
		}

		for _, pkt := range packets {
			if waitingForKeyframe && pkt.ContFlags != 0 {
				sent = pkt
				continue
			}
			waitingForKeyframe = false

			o.conn.SetWriteDeadline(time.Now().Add(directWriteTimeout))
			if _, err := o.conn.Write(pkt.Data); err != nil {
				return
			}
			sent = pkt
		}
	}
}

// sanitizeHeaderValue removes CR and LF characters from s to prevent
// HTTP header injection.
func sanitizeHeaderValue(s string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(s)
}
