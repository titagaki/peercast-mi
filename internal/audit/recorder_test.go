package audit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type memoryBackend struct {
	mu         sync.Mutex
	down       bool
	events     map[string]Event
	expired    int
	recoveries int
}

func (m *memoryBackend) Ready(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.down {
		return errors.New("offline")
	}
	return nil
}
func (m *memoryBackend) Write(_ context.Context, e Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.down {
		return errors.New("offline")
	}
	if e.Expired {
		m.expired++
		return nil
	}
	if m.events == nil {
		m.events = map[string]Event{}
	}
	m.events[e.ID] = e
	return nil
}
func (m *memoryBackend) Recover(context.Context, string, string) ([]Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recoveries++
	return nil, nil
}
func (m *memoryBackend) Prune(context.Context, time.Time) error { return nil }
func (m *memoryBackend) count(kind string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, e := range m.events {
		if e.Type == kind {
			n++
		}
	}
	return n
}
func until(t *testing.T, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not reached")
}
func closeRecorder(t *testing.T, r *Recorder) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := r.Close(ctx); err != nil {
		t.Fatal(err)
	}
}
func fixtureEvent(kind string) Event {
	return Event{ID: ID(), Node: "test", Boot: ID(), Seq: 1, At: time.Now().UTC(), Type: kind, Actor: Actor{Source: "system"}, Outcome: "success", Version: 1}
}

func TestSpoolOfflineRestartAndIdempotency(t *testing.T) {
	dir := t.TempDir()
	db := &memoryBackend{down: true}
	r := New(Options{Node: "test", Dir: dir}, db)
	r.Emit(Event{Type: "auth.login", Actor: Actor{Account: "site:x:1", IP: "[2001:db8::1]:123", Source: "site"}, Outcome: "success"})
	closeRecorder(t, r)
	files, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if len(files) == 0 {
		t.Fatal("no durable spool")
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	// Emulate commit success followed by a lost acknowledgement: retain a copy.
	if err = os.WriteFile(filepath.Join(dir, "duplicate.jsonl"), data, 0600); err != nil {
		t.Fatal(err)
	}
	db.mu.Lock()
	db.down = false
	db.mu.Unlock()
	r2 := New(Options{Node: "test", Dir: dir}, db)
	defer closeRecorder(t, r2)
	until(t, func() bool { return db.count("auth.login") == 1 && r2.Status().PendingFiles == 0 })
	db.mu.Lock()
	defer db.mu.Unlock()
	for _, e := range db.events {
		if e.Type == "auth.login" && e.Actor.IP != "2001:db8::1" {
			t.Fatal(e.Actor)
		}
	}
}
func TestCorruptAndOversizeSpoolDoesNotHideLaterEvents(t *testing.T) {
	dir := t.TempDir()
	a, b := fixtureEvent("auth.logout"), fixtureEvent("auth.logout")
	raw := string(mustJSON(a)) + "\n" + strings.Repeat("x", MaxEventBytes*2) + "\n" + string(mustJSON(b)) + "\n{\"partial\":"
	if err := os.WriteFile(filepath.Join(dir, ".batch-crashed.tmp"), []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	db := &memoryBackend{}
	r := New(Options{Node: "test", Dir: dir}, db)
	defer closeRecorder(t, r)
	until(t, func() bool { return db.count("auth.logout") == 2 && r.Status().Dropped > 0 })
	if !r.Status().Degraded {
		t.Fatal("corruption must remain visible")
	}
	paths, _ := filepath.Glob(filepath.Join(dir, "*.bad"))
	if len(paths) != 1 {
		t.Fatal(paths)
	}
}
func TestBoundedQueueAndSpool(t *testing.T) {
	// No worker: deterministic queue pressure, with the normal producer path.
	r := &Recorder{opt: Options{Node: "test"}, boot: ID(), queue: make(chan Event, 1), failures: map[string]*failureCount{}}
	r.Emit(Event{Type: "auth.logout", Outcome: "success"})
	r.Emit(Event{Type: "auth.logout", Outcome: "success"})
	if r.Status().Dropped != 1 {
		t.Fatal(r.Status())
	}
	<-r.queue
	r.aggregates(false)
	e := <-r.queue
	if e.Type != "audit.gap" || e.Payload.Count != 1 {
		t.Fatal(e)
	}
	dir := t.TempDir()
	worker := New(Options{Node: "test", Dir: dir, MaxBytes: 1}, &memoryBackend{down: true})
	worker.Emit(Event{Type: "auth.logout", Outcome: "success"})
	closeRecorder(t, worker)
	if worker.Status().Dropped == 0 {
		t.Fatal("spool limit ignored")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if len(files) != 0 {
		t.Fatal(files)
	}
}
func TestFailureAggregationAndLock(t *testing.T) {
	r := &Recorder{opt: Options{Node: "test"}, boot: ID(), queue: make(chan Event, 8), failures: map[string]*failureCount{}}
	for i := 0; i < 20; i++ {
		r.Emit(Event{Type: "rtmp.publish", Actor: Actor{Source: "rtmp", IP: "192.0.2.1:3"}, Outcome: "failure", Reason: "unknown_key"})
	}
	if len(r.queue) != 0 {
		t.Fatal("failures must aggregate")
	}
	r.aggregates(true)
	e := <-r.queue
	if e.Payload.Count != 20 || e.Payload.FirstAt == nil || e.Actor.IP != "192.0.2.1" {
		t.Fatal(e)
	}
	dir := t.TempDir()
	one := &Recorder{opt: Options{Dir: dir}}
	if err := one.acquire(); err != nil {
		t.Fatal(err)
	}
	defer one.lock.Close()
	two := New(Options{Node: "test", Dir: dir}, &memoryBackend{})
	closeRecorder(t, two)
	if two.Status().Reason != "spool_unavailable" || two.Status().Dropped == 0 {
		t.Fatal(two.Status())
	}
}
func TestExpiredLifecycleOnlyUsedForReconciliation(t *testing.T) {
	dir := t.TempDir()
	old := fixtureEvent("auth.login")
	old.At = time.Now().Add(-100 * 24 * time.Hour)
	lifecycle := fixtureEvent("broadcast.create")
	lifecycle.At = old.At
	lifecycle.Payload.Broadcast = &Broadcast{ID: ID(), ChannelID: ID(), Created: old.At, Revision: 1}
	if err := os.WriteFile(filepath.Join(dir, "old.jsonl"), append(append(mustJSON(old), '\n'), append(mustJSON(lifecycle), '\n')...), 0600); err != nil {
		t.Fatal(err)
	}
	db := &memoryBackend{}
	r := New(Options{Node: "test", Dir: dir}, db)
	defer closeRecorder(t, r)
	until(t, func() bool { db.mu.Lock(); defer db.mu.Unlock(); return db.expired == 1 && db.recoveries > 0 })
	if db.count("auth.login") != 0 {
		t.Fatal("expired raw event retained")
	}
}

// A partially drained batch must progress even if processing the whole batch
// takes longer than a single database deadline.
type limitedBackend struct {
	memoryBackend
	budget int
	calls  int
}

func (m *limitedBackend) Write(ctx context.Context, e Event) error {
	m.calls++
	if m.calls > m.budget {
		return context.DeadlineExceeded
	}
	return m.memoryBackend.Write(ctx, e)
}
func TestPartialBatchProgress(t *testing.T) {
	dir := t.TempDir()
	var data []byte
	for i := 0; i < 5; i++ {
		e := fixtureEvent("auth.logout")
		data = append(data, mustJSON(e)...)
		data = append(data, '\n')
	}
	if err := os.WriteFile(filepath.Join(dir, "batch.jsonl"), data, 0600); err != nil {
		t.Fatal(err)
	}
	db := &limitedBackend{budget: 2}
	r := &Recorder{opt: Options{Dir: dir, Retention: 90 * 24 * time.Hour}, backend: db}
	for i := 0; i < 3; i++ {
		db.calls = 0
		err := r.deliver(context.Background())
		if i < 2 && !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
		if i == 2 && err != nil {
			t.Fatal(err)
		}
	}
	if db.count("auth.logout") != 5 {
		t.Fatal("batch did not progress")
	}
	paths, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if len(paths) != 0 {
		t.Fatal(paths)
	}
}

func TestUnconfiguredDatabaseKeepsDurableEvents(t *testing.T) {
	var db *MySQL
	dir := t.TempDir()
	r := New(Options{Node: "test", Dir: dir}, db)
	r.Emit(Event{Type: "auth.login", Outcome: "success", Actor: Actor{Source: "site"}})
	until(t, func() bool { return r.Status().Degraded })
	closeRecorder(t, r)
	files, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if len(files) == 0 || r.Status().Dropped != 0 {
		t.Fatal("unconfigured DB lost events", r.Status())
	}
}
