// Package catalog fetches operator-configured YP channel directories, separately
// from the PCP announcement connection. Callers share one bounded cache.
package catalog

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/titagaki/peercast-mi/internal/config"
)

const cacheTTL = time.Minute
const staleTTL = 5 * time.Minute
const maxBytes = 4 << 20

// Channel uses the field names consumed by peca-live's updateYPChannels client.
type Channel struct {
	YellowPage  string `json:"yellowPage"`
	Name        string `json:"name"`
	ChannelID   string `json:"channelId"`
	Tracker     string `json:"tracker"`
	ContactURL  string `json:"contactUrl"`
	Genre       string `json:"genre"`
	Description string `json:"description"`
	Comment     string `json:"comment"`
	ContentType string `json:"contentType"`
	Creator     string `json:"creator"`
	Album       string `json:"album"`
	TrackTitle  string `json:"trackTitle"`
	TrackURL    string `json:"trackUrl"`
	Listeners   *int   `json:"listeners"`
	Relays      *int   `json:"relays"`
	Bitrate     *int   `json:"bitrate"`
	Uptime      *int   `json:"uptime"`
}
type Status struct {
	Name       string     `json:"name"`
	Configured bool       `json:"configured"`
	Error      string     `json:"error,omitempty"`
	Stale      bool       `json:"stale"`
	UpdatedAt  *time.Time `json:"updatedAt,omitempty"`
}
type source struct {
	config.YP
	rows    []Channel
	updated time.Time
	err     string
}
type Catalog struct {
	mu       sync.Mutex
	sources  []source
	next     time.Time
	inflight chan struct{}
	client   *http.Client
}

func New(entries []config.YP) *Catalog {
	c := &Catalog{client: &http.Client{Timeout: 5 * time.Second,
		Transport: &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 3 * time.Second}).DialContext, ResponseHeaderTimeout: 4 * time.Second},
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if len(via) >= 5 || r.URL.User != nil || (r.URL.Scheme != "http" && r.URL.Scheme != "https") {
				return fmt.Errorf("invalid redirect")
			}
			return nil
		},
	}}
	for _, entry := range entries {
		c.sources = append(c.sources, source{YP: entry})
	}
	return c
}

// Update coalesces concurrent refreshes, including failures. A caller may cancel
// waiting without cancelling the bounded refresh shared by other callers.
func (c *Catalog) Update(ctx context.Context) ([]Channel, []Status, error) {
	c.mu.Lock()
	if c.inflight == nil && !time.Now().Before(c.next) {
		c.inflight = make(chan struct{})
		go c.refresh()
	}
	done := c.inflight
	c.mu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		}
	}
	rows, status := c.Snapshot()
	return rows, status, nil
}
func (c *Catalog) refresh() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i := range c.sources {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c.mu.Lock()
			entry := c.sources[i].YP
			c.mu.Unlock()
			if entry.ChannelsURL == "" {
				return
			}
			var rows []Channel
			var err error
			select {
			case sem <- struct{}{}:
				rows, err = c.fetch(ctx, entry)
				<-sem
			case <-ctx.Done():
				err = ctx.Err()
			}
			c.mu.Lock()
			defer c.mu.Unlock()
			s := &c.sources[i]
			if err != nil {
				s.err = "番組一覧を取得できません。管理者は channels_url と接続先を確認してください。"
				return
			}
			s.rows, s.updated, s.err = rows, time.Now(), ""
		}(i)
	}
	wg.Wait()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.next = time.Now().Add(cacheTTL)
	close(c.inflight)
	c.inflight = nil
}
func (c *Catalog) Snapshot() ([]Channel, []Status) {
	c.mu.Lock()
	defer c.mu.Unlock()
	rows := make([]Channel, 0)
	statuses := make([]Status, 0, len(c.sources))
	for _, s := range c.sources {
		status := Status{Name: s.Name, Configured: s.ChannelsURL != "", Error: s.err}
		if !s.updated.IsZero() {
			updated := s.updated
			status.UpdatedAt = &updated
		}
		if time.Since(s.updated) < staleTTL {
			rows = append(rows, s.rows...)
			status.Stale = s.err != ""
		}
		statuses = append(statuses, status)
	}
	return rows, statuses
}
func (c *Catalog) fetch(ctx context.Context, entry config.YP) ([]Channel, error) {
	u, err := url.Parse(entry.ChannelsURL)
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("invalid channels_url")
	}
	r, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	r.Header.Set("Accept", "text/plain")
	resp, err := c.client.Do(r)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/html") {
		return nil, fmt.Errorf("unexpected directory response")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxBytes {
		return nil, fmt.Errorf("directory too large")
	}
	return parse(string(data), entry.Name)
}
func integer(s string) *int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return nil
	}
	return &n
}
func uptime(s string) *int {
	p := strings.Split(s, ":")
	if len(p) == 1 {
		return integer(s)
	}
	if len(p) != 2 {
		return nil
	}
	h, m := integer(p[0]), integer(p[1])
	if h == nil || m == nil || *h < 0 || *h > 1000000 || *m < 0 || *m > 59 {
		return nil
	}
	n := (*h*60 + *m) * 60
	return &n
}
func parse(body, name string) ([]Channel, error) {
	if !utf8.ValidString(body) {
		return nil, fmt.Errorf("directory is not UTF-8")
	}
	rows := make([]Channel, 0)
	scanner := bufio.NewScanner(strings.NewReader(strings.TrimPrefix(body, "\ufeff")))
	scanner.Buffer(make([]byte, 4096), 64<<10)
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		p := strings.Split(line, "<>")
		if len(p) < 10 {
			return nil, fmt.Errorf("invalid index.txt row")
		}
		id, err := hex.DecodeString(p[1])
		if err != nil || len(id) != 16 {
			continue
		}
		get := func(i int) string {
			if i >= len(p) {
				return ""
			}
			return html.UnescapeString(p[i])
		}
		rows = append(rows, Channel{YellowPage: name, Name: get(0), ChannelID: strings.ToUpper(p[1]), Tracker: get(2), ContactURL: get(3), Genre: get(4), Description: get(5), Listeners: integer(p[6]), Relays: integer(p[7]), Bitrate: integer(p[8]), ContentType: get(9), Creator: get(10), Album: get(11), TrackTitle: get(12), TrackURL: get(13), Uptime: uptime(get(15)), Comment: get(17)})
		if len(rows) > 10000 {
			return nil, fmt.Errorf("too many channels")
		}
	}
	return rows, scanner.Err()
}

// PublicTracker only permits literal public unicast IPs from the directory.
// No DNS resolution (and thus no DNS rebinding) or browser-supplied tip occurs.
func PublicTracker(addr string) bool {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	// IsGlobalUnicast alone also accepts special-use IPv4 ranges, including
	// shared carrier space used by some cloud metadata endpoints.
	if v4 := ip.To4(); v4 != nil {
		if v4[0] == 0 || v4[0] >= 240 || (v4[0] == 100 && v4[1]&0xc0 == 64) || (v4[0] == 198 && v4[1]&0xfe == 18) {
			return false
		}
	}
	p, err := strconv.Atoi(port)
	return err == nil && p > 0 && p <= 65535 && ip != nil && ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast()
}
