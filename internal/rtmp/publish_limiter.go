package rtmp

import (
	"net"
	"sync"
	"time"
)

// Shared by all encoder connections; reconnecting cannot reset the counters.
// Only two-letter/four-digit key attempts use this limiter. Established media is unaffected.
type publishLimiter struct {
	mu    sync.Mutex
	now   func() time.Time
	until time.Time
	total int
	byIP  map[string]int
}

func newPublishLimiter() *publishLimiter { return &publishLimiter{now: time.Now} }

func (l *publishLimiter) allow(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	if ip := net.ParseIP(host); ip != nil {
		host = ip.String()
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if !now.Before(l.until) {
		l.until = now.Add(time.Minute)
		l.total = 0
		l.byIP = make(map[string]int)
	}
	if l.total >= 120 || l.byIP[host] >= 10 {
		return false
	}
	l.total++
	l.byIP[host]++
	return true
}

func isShortStreamKey(key string) bool {
	if len(key) != 6 {
		return false
	}
	for i := 0; i < 2; i++ {
		if key[i] < 'a' || key[i] > 'z' {
			return false
		}
	}
	for i := 2; i < 6; i++ {
		if key[i] < '0' || key[i] > '9' {
			return false
		}
	}
	return true
}
