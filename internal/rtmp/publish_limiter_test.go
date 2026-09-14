package rtmp

import (
	"fmt"
	"testing"
	"time"

	"github.com/titagaki/peercast-mi/internal/channel"
	"github.com/titagaki/peercast-pcp/pcp"
	"github.com/yutopp/go-rtmp/message"
)

func TestShortKeyAttemptsShareIPBudgetAcrossConnections(t *testing.T) {
	mgr := channel.NewManager(pcp.GnuID{})
	if err := mgr.IssueStreamKey("mobile", "ab1234"); err != nil {
		t.Fatal(err)
	}
	limiter := newPublishLimiter()
	now := time.Unix(1000, 0)
	limiter.now = func() time.Time { return now }
	publish := func(key string, port int) error {
		h := newHandler(mgr, fmt.Sprintf("192.0.2.1:%d", port))
		h.publishLimiter = limiter
		return h.OnPublish(nil, 0, &message.NetStreamPublish{PublishingName: key})
	}
	for i := 0; i < 10; i++ {
		if publish("zz4321", 1000+i) == nil {
			t.Fatal("unknown key accepted")
		}
	}
	if publish("ab1234", 2000) == nil {
		t.Fatal("reconnect bypassed IP limit")
	}
	now = now.Add(time.Minute)
	if err := publish("ab1234", 2001); err != nil {
		t.Fatal(err)
	}
	if err := mgr.IssueStreamKey("legacy", "long-legacy-key"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 15; i++ {
		if err := publish("long-legacy-key", 3000+i); err != nil {
			t.Fatal("legacy limited", err)
		}
	}
}

func TestShortKeyGlobalBudgetAndWindowReset(t *testing.T) {
	l := newPublishLimiter()
	now := time.Unix(1000, 0)
	l.now = func() time.Time { return now }
	for i := 0; i < 120; i++ {
		if !l.allow(fmt.Sprintf("192.0.2.%d:1935", i)) {
			t.Fatal(i)
		}
	}
	if l.allow("198.51.100.1:1935") {
		t.Fatal("global limit bypassed")
	}
	if len(l.byIP) > 120 {
		t.Fatal("unbounded tracking")
	}
	now = now.Add(time.Minute)
	if !l.allow("198.51.100.1:1935") || l.total != 1 || len(l.byIP) != 1 {
		t.Fatal("window not reset")
	}
}

func TestShortKeyShape(t *testing.T) {
	for _, key := range []string{"ab1234", "aa0000", "zz9999"} {
		if !isShortStreamKey(key) {
			t.Fatal(key)
		}
	}
	for _, key := range []string{"123456", "AB1234", "ab123", "abc123", "ab12345", "é1234", "ab12x4", "long-legacy-key"} {
		if isShortStreamKey(key) {
			t.Fatal(key)
		}
	}
}
