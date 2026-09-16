package rtmp

import (
	"bytes"
	"github.com/titagaki/peercast-mi/internal/audit"
	"github.com/titagaki/peercast-mi/internal/channel"
	"github.com/titagaki/peercast-mi/internal/id"
	"github.com/yutopp/go-rtmp/message"
	"sync"
	"sync/atomic"
	"testing"
)

type auditCapture struct {
	mu     sync.Mutex
	events []audit.Event
}

func (c *auditCapture) Emit(e audit.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
}
func TestAuditMediaAndRecreatedChannel(t *testing.T) {
	m := channel.NewManager(id.NewRandom())
	log := &auditCapture{}
	m.Audit = log
	defer m.StopAll()
	const key = "secret-key-never-record"
	if err := m.IssueStreamKey("owner", key); err != nil {
		t.Fatal(err)
	}
	h := newHandler(m, "[2001:db8::1]:5000")
	if err := h.OnPublish(nil, 0, &message.NetStreamPublish{PublishingName: key}); err != nil {
		t.Fatal(err)
	}
	info := channel.ChannelInfo{Name: "test", Type: "FLV"}
	first, err := m.Broadcast(key, info, channel.TrackInfo{})
	if err != nil {
		t.Fatal(err)
	}
	if err = h.OnVideo(0, bytes.NewReader([]byte{0x17, 0, 0, 0, 0, 1})); err != nil {
		t.Fatal(err)
	}
	if err = h.OnAudio(0, bytes.NewReader([]byte{0xaf, 0, 1, 2})); err != nil {
		t.Fatal(err)
	}
	if err = h.OnVideo(0, bytes.NewReader([]byte{0x17, 2, 0, 0, 0})); err != nil {
		t.Fatal(err)
	}
	if first.AuditRun.Snapshot().FirstMedia != nil {
		t.Fatal("sequence headers counted as media")
	}
	if err = h.OnVideo(1, bytes.NewReader([]byte{0x17, 1, 0, 0, 0, 1})); err != nil {
		t.Fatal(err)
	}
	if first.AuditRun.Snapshot().FirstMedia == nil {
		t.Fatal("actual media missing")
	}
	m.StopInstance(first, audit.Actor{Source: "site"}, "user_stop")
	second, err := m.Broadcast(key, info, channel.TrackInfo{})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || first.AuditRun.Snapshot().ID == second.AuditRun.Snapshot().ID {
		t.Fatal("run identity")
	}
	h.OnClose()
	if _, ok := m.GetByID(second.ID); !ok {
		t.Fatal("old encoder stopped new channel")
	}
	h2 := newHandler(m, "192.0.2.2:5000")
	h2.streamKey = key
	h2.writeData(makeFLVTag(8, 1, []byte{0xaf, 1, 1}), 4)
	m.StopInstance(second, audit.Actor{Source: "site"}, "user_stop")
	third, err := m.Broadcast(key, info, channel.TrackInfo{})
	if err != nil {
		t.Fatal(err)
	}
	h2.writeData(makeFLVTag(8, 2, []byte{0xaf, 1, 2}), 4)
	if third.AuditRun.Snapshot().FirstMedia == nil || h2.inputChannel != third {
		t.Fatal("encoder did not move to new run")
	}
	var closing atomic.Bool
	closing.Store(true)
	h2.shuttingDown = &closing
	h2.OnClose()
	if third.AuditRun.Snapshot().Reason != "server_shutdown" {
		t.Fatal("shutdown classified as encoder failure")
	}
	starts := map[string]string{}
	for _, e := range log.events {
		if e.Type == "input.start" {
			starts[e.BroadcastID] = e.ConnectionID
		}
	}
	if len(starts) != 3 || starts[second.AuditRun.Snapshot().ID] != starts[third.AuditRun.Snapshot().ID] {
		t.Fatal(starts)
	}
}
