package catalog

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/titagaki/peercast-mi/internal/config"
)

const sample = "日本語 &amp; 音楽<>0123456789abcdef0123456789abcdef<>8.8.8.8:7144<>https://example.test/bbs<>ゲーム<>テスト &lt;Open&gt;<>-1<>-1<>1500<>FLV<>artist<>album<>title<>https://example.test/track<>encoded<>12:34<>click<>hello<>1"

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"text/plain; charset=UTF-8"}}, Body: io.NopCloser(strings.NewReader(body))}
}

// Optional live check is separate from deterministic regression tests.
func TestLiveDirectory(t *testing.T) {
	u := os.Getenv("PEERCAST_TEST_YP_URL")
	if u == "" {
		t.Skip("set PEERCAST_TEST_YP_URL for a live directory read")
	}
	c := New([]config.YP{{Name: "live", ChannelsURL: u}})
	rows, statuses, err := c.Update(context.Background())
	if err != nil || statuses[0].Error != "" {
		t.Fatal(statuses, err)
	}
	n := 0
	for _, row := range rows {
		if row.ChannelID != strings.Repeat("0", 32) && PublicTracker(row.Tracker) {
			n++
		}
	}
	t.Logf("parsed %d rows; %d non-notice public trackers", len(rows), n)
}

func TestParseYP4G(t *testing.T) {
	rows, err := parse("\ufeff"+sample+"\r\n\n", "YP")
	if err != nil || len(rows) != 1 {
		t.Fatalf("%v %v", rows, err)
	}
	r := rows[0]
	if r.Name != "日本語 & 音楽" || r.Description != "テスト <Open>" || r.ChannelID != "0123456789ABCDEF0123456789ABCDEF" || r.YellowPage != "YP" || r.Uptime == nil || *r.Uptime != (12*60+34)*60 || r.Listeners == nil || *r.Listeners != -1 || r.Creator != "artist" || r.Comment != "hello" {
		t.Fatalf("%+v", r)
	}
	for _, invalid := range []string{"<!doctype html>", "\xff", strings.Repeat("x", 70<<10)} {
		if _, err := parse(invalid, "YP"); err == nil {
			t.Fatalf("accepted invalid directory")
		}
	}
	rows, err = parse(strings.Replace(sample, "0123456789abcdef0123456789abcdef", "invalid", 1), "YP")
	if err != nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
	rows, err = parse(strings.Replace(sample, "0123456789abcdef0123456789abcdef", strings.Repeat("0", 32), 1), "YP")
	if err != nil || len(rows) != 1 {
		t.Fatal("RPC retains zero-ID notice rows", err)
	}
	for _, v := range []string{"", "abc", "12:abc", "-1:01", "2:70", "999999999999:00", "1:2:3"} {
		if uptime(v) != nil {
			t.Fatalf("accepted uptime %q", v)
		}
	}
}

func TestCacheCoalescingFailureAndExpiry(t *testing.T) {
	c := New([]config.YP{{Name: "ok", ChannelsURL: "https://yp.test/index.txt"}, {Name: "disabled"}})
	var calls atomic.Int32
	var fail atomic.Bool
	c.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Error("credentials forwarded")
		}
		if fail.Load() {
			return response(503, "unavailable"), nil
		}
		return response(200, sample), nil
	})
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rows, statuses, err := c.Update(context.Background())
			if err != nil || len(rows) != 1 || len(statuses) != 2 {
				t.Error(rows, statuses, err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal("refresh stampede", calls.Load())
	}
	fail.Store(true)
	c.mu.Lock()
	c.next = time.Time{}
	c.mu.Unlock()
	rows, status, err := c.Update(context.Background())
	if err != nil || len(rows) != 1 || !status[0].Stale || status[0].Error == "" || status[1].Configured {
		t.Fatal(rows, status, err)
	}
	c.Update(context.Background())
	if calls.Load() != 2 {
		t.Fatal("failures must also be cached")
	}
	c.mu.Lock()
	c.sources[0].updated = time.Now().Add(-staleTTL - time.Second)
	c.mu.Unlock()
	rows, status = c.Snapshot()
	if len(rows) != 0 || status[0].Stale {
		t.Fatal("expired stale rows served")
	}
	c.mu.Lock()
	c.next = time.Time{}
	c.mu.Unlock()
	fail.Store(false)
	rows, status, err = c.Update(context.Background())
	if err != nil || len(rows) != 1 || status[0].Error != "" {
		t.Fatal("failed to recover")
	}
}

func TestPartialFailureAndBounds(t *testing.T) {
	c := New([]config.YP{{Name: "ok", ChannelsURL: "https://ok.test/index.txt"}, {Name: "bad", ChannelsURL: "https://bad.test/index.txt"}})
	c.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "bad.test" {
			return response(500, ""), nil
		}
		return response(200, sample), nil
	})
	rows, status, err := c.Update(context.Background())
	if err != nil || len(rows) != 1 || status[0].Error != "" || status[1].Error == "" {
		t.Fatal(rows, status, err)
	}
	for _, body := range []string{"<html>error</html>", strings.Repeat("x", maxBytes+1)} {
		c.client.Transport = roundTrip(func(*http.Request) (*http.Response, error) { return response(200, body), nil })
		if _, err := c.fetch(context.Background(), config.YP{ChannelsURL: "https://yp.test/index.txt"}); err == nil {
			t.Fatal("invalid response accepted")
		}
	}
	if _, err := c.fetch(context.Background(), config.YP{ChannelsURL: "file:///etc/passwd"}); err == nil {
		t.Fatal("non-HTTP source accepted")
	}
}

func TestCancelledWaitDoesNotCancelSharedRefresh(t *testing.T) {
	c := New([]config.YP{{Name: "YP", ChannelsURL: "https://yp.test/index.txt"}})
	entered, release := make(chan struct{}), make(chan struct{})
	c.client.Transport = roundTrip(func(*http.Request) (*http.Response, error) {
		close(entered)
		<-release
		return response(200, sample), nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, _, err := c.Update(ctx); done <- err }()
	<-entered
	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatal(err)
	}
	close(release)
	rows, _, err := c.Update(context.Background())
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
}

func TestPublicTracker(t *testing.T) {
	for _, addr := range []string{"100.100.100.200:80", "0.1.2.3:80", "240.0.0.1:80", "198.18.0.1:80"} {
		if PublicTracker(addr) {
			t.Fatal(addr)
		}
	}
	for _, addr := range []string{"8.8.8.8:7144", "[2001:4860:4860::8888]:7144"} {
		if !PublicTracker(addr) {
			t.Fatal(addr)
		}
	}
	for _, addr := range []string{"", "127.0.0.1:7144", "10.0.0.1:7144", "169.254.169.254:80", "192.168.0.1:80", "[::1]:80", "[fc00::1]:80", "[fe80::1]:80", "[::ffff:127.0.0.1]:80", "0.0.0.0:80", "224.0.0.1:80", "evil.test:80", "8.8.8.8:0", "8.8.8.8:65536", "http://8.8.8.8:80"} {
		if PublicTracker(addr) {
			t.Fatal(addr)
		}
	}
}
