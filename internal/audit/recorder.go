package audit

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

const MaxEventBytes = 64 << 10

type Backend interface {
	Ready(context.Context) error
	Write(context.Context, Event) error
	Recover(context.Context, string, string) ([]Event, error)
	Prune(context.Context, time.Time) error
}
type Options struct {
	Node, Dir string
	QueueSize int
	MaxBytes  int64
	Retention time.Duration // Zero disables expiry and pruning.
}
type Status struct {
	Enabled          bool       `json:"enabled"`
	Degraded         bool       `json:"degraded"`
	LastSuccess      *time.Time `json:"lastSuccess,omitempty"`
	PendingBytes     int64      `json:"pendingBytes"`
	PendingFiles     int        `json:"pendingFiles"`
	QuarantinedFiles int        `json:"quarantinedFiles"`
	Queued           int        `json:"queued"`
	Dropped          uint64     `json:"dropped"`
	Reason           string     `json:"reason,omitempty"`
}
type failureCount struct {
	e           Event
	count       uint64
	first, last time.Time
}
type Recorder struct {
	opt      Options
	backend  Backend
	boot     string
	queue    chan Event
	done     chan struct{}
	stop     chan struct{}
	once     sync.Once
	mu       sync.Mutex
	seq      uint64
	status   Status
	closed   bool
	failures map[string]*failureCount
	gap      uint64
	lock     *os.File
	offsets  map[string]int
	badPaths map[string]bool
}

func New(opt Options, b Backend) *Recorder {
	if opt.QueueSize <= 0 {
		opt.QueueSize = 4096
	}
	if opt.MaxBytes <= 0 {
		opt.MaxBytes = 512 << 20
	}
	r := &Recorder{opt: opt, backend: b, boot: ID(), queue: make(chan Event, opt.QueueSize), stop: make(chan struct{}), done: make(chan struct{}), failures: map[string]*failureCount{}, status: Status{Enabled: true}}
	go r.loop()
	r.Emit(Event{Type: "system.start", Outcome: "success"})
	return r
}
func (r *Recorder) Emit(e Event) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	e = canonical(e)
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	if (e.Type == "auth.login" || e.Type == "rtmp.publish") && e.Outcome != "success" {
		key := e.Type + "|" + e.Actor.IP + "|" + e.Reason
		if f := r.failures[key]; f != nil {
			f.count++
			f.last = e.At
			return
		}
		if len(r.failures) < 1024 {
			r.failures[key] = &failureCount{e: e, count: 1, first: e.At, last: e.At}
		} else {
			r.dropLocked("failure_buckets_full", 1)
		}
		return
	}
	r.enqueueLocked(e)
}
func (r *Recorder) stamp(e Event) Event {
	r.seq++
	e.ID = ID()
	e.Node = r.opt.Node
	e.Boot = r.boot
	e.Seq = r.seq
	e.Version = 1
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	return canonical(e)
}
func (r *Recorder) enqueueLocked(e Event) {
	e = r.stamp(e)
	if r.status.Dropped > 0 {
		if e.Payload.Broadcast != nil {
			b := *e.Payload.Broadcast
			b.Incomplete = true
			e.Payload.Broadcast = &b
		}
		if e.Payload.Input != nil {
			in := *e.Payload.Input
			in.Incomplete = true
			e.Payload.Input = &in
		}
	}
	if len(mustJSON(e)) > MaxEventBytes {
		r.dropLocked("event_too_large", 1)
		return
	}
	select {
	case r.queue <- e:
	default:
		r.dropLocked("queue_full", 1)
	}
}
func (r *Recorder) dropLocked(reason string, n uint64) {
	r.status.Dropped += n
	r.gap += n
	r.status.Degraded = true
	r.status.Reason = reason
}
func (r *Recorder) fail(reason string) {
	r.mu.Lock()
	changed := r.status.Reason != reason
	r.status.Degraded = true
	r.status.Reason = reason
	r.mu.Unlock()
	if changed {
		slog.Error("audit: recording degraded", "reason", reason)
	}
}
func (r *Recorder) Status() Status {
	if r == nil {
		return Status{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.status
	s.Queued = len(r.queue)
	return s
}
func (r *Recorder) Close(ctx context.Context) error {
	if r == nil {
		return nil
	}
	r.once.Do(func() {
		r.Emit(Event{Type: "system.stop", Outcome: "success"})
		r.mu.Lock()
		r.closed = true
		r.mu.Unlock()
		close(r.stop)
	})
	select {
	case <-r.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (r *Recorder) aggregates(all bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	for k, f := range r.failures {
		if all || now.Sub(f.first) >= time.Minute {
			e := f.e
			e.Payload.Count = f.count
			e.Payload.FirstAt = &f.first
			e.Payload.LastAt = &f.last
			r.enqueueLocked(e)
			delete(r.failures, k)
		}
	}
	if r.gap > 0 && len(r.queue) < cap(r.queue) {
		slog.Warn("audit: events dropped", "count", r.gap)
		n := r.gap
		r.gap = 0
		r.enqueueLocked(Event{Type: "audit.gap", Outcome: "failure", Reason: "events_dropped", Payload: Payload{Count: n}})
	}
}
func (r *Recorder) files() ([]string, int64, error) {
	paths, err := filepath.Glob(filepath.Join(r.opt.Dir, "*.jsonl"))
	if err != nil {
		return nil, 0, err
	}
	sort.Strings(paths)
	var size int64
	quarantined := 0
	entries, err := os.ReadDir(r.opt.Dir)
	if err != nil {
		return nil, 0, err
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".bad") {
			quarantined++
		}
		if strings.HasSuffix(e.Name(), ".jsonl") || strings.HasSuffix(e.Name(), ".bad") || strings.HasSuffix(e.Name(), ".tmp") {
			i, er := e.Info()
			if er != nil {
				return nil, 0, er
			}
			size += i.Size()
		}
	}
	r.mu.Lock()
	r.status.PendingFiles = len(paths)
	r.status.PendingBytes = size
	r.status.QuarantinedFiles = quarantined
	r.mu.Unlock()
	return paths, size, nil
}
func (r *Recorder) appendBatch(batch []Event) error {
	if len(batch) == 0 {
		return nil
	}
	_, size, err := r.files()
	if err != nil {
		return err
	}
	var data []byte
	for _, e := range batch {
		data = append(data, mustJSON(e)...)
		data = append(data, '\n')
	}
	if size+int64(len(data)) > r.opt.MaxBytes {
		return errors.New("spool limit")
	}
	f, err := os.CreateTemp(r.opt.Dir, ".batch-*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err != nil {
		return err
	}
	if ce != nil {
		return ce
	}
	target := filepath.Join(r.opt.Dir, fmt.Sprintf("%020d-%s-%020d.jsonl", time.Now().UnixNano(), r.boot, batch[0].Seq))
	if err = os.Rename(name, target); err != nil {
		return err
	}
	return syncDir(r.opt.Dir)
}
func syncDir(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}
func (r *Recorder) acquire() error {
	if err := os.MkdirAll(r.opt.Dir, 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(r.opt.Dir, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	if err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		return err
	}
	// Recover the complete lines in a batch interrupted before its rename.
	paths, err := filepath.Glob(filepath.Join(r.opt.Dir, ".batch-*.tmp"))
	if err == nil {
		for _, p := range paths {
			if err = os.Rename(p, filepath.Join(r.opt.Dir, "00000000000000000000-recovered-"+ID()+".jsonl")); err != nil {
				break
			}
		}
	}
	if err == nil {
		err = syncDir(r.opt.Dir)
	}
	if err != nil {
		f.Close()
		return err
	}
	r.lock = f
	return nil
}
func (r *Recorder) loop() {
	defer close(r.done)
	defer func() {
		if r.lock != nil {
			r.lock.Close()
		}
	}()
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	var next time.Time
	backoff := time.Second
	recovered := false
	lastPrune := time.Time{}
	for {
		stopping := false
		select {
		case <-r.stop:
			stopping = true
		case <-tick.C:
		}
		r.aggregates(stopping)
		if r.lock == nil {
			if err := r.acquire(); err != nil {
				r.fail("spool_unavailable")
				r.discardQueue()
				if stopping {
					return
				}
				continue
			}
		}
		batch := make([]Event, 0, 256)
		for {
			select {
			case e := <-r.queue:
				batch = append(batch, e)
			default:
				goto drained
			}
			if len(batch) == 256 {
				break
			}
		}
	drained:
		if err := r.appendBatch(batch); err != nil {
			r.fail("spool_write_failed")
			r.mu.Lock()
			r.dropLocked("spool_write_failed", uint64(len(batch)))
			r.mu.Unlock()
		}
		if stopping {
			if len(r.queue) > 0 {
				continue
			}
			return
		}
		if time.Now().Before(next) {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err := r.backend.Ready(ctx)
		if err == nil {
			err = r.deliver(ctx)
		}
		if err == nil && !recovered {
			var events []Event
			events, err = r.backend.Recover(ctx, r.opt.Node, r.boot)
			if err == nil {
				r.mu.Lock()
				for i := range events {
					events[i] = r.stamp(events[i])
				}
				r.mu.Unlock()
				err = r.appendBatch(events)
				recovered = len(events) == 0
			}
		}
		if err == nil && r.opt.Retention > 0 && time.Since(lastPrune) > time.Minute {
			err = r.backend.Prune(ctx, time.Now().UTC().Add(-r.opt.Retention))
			if err == nil {
				lastPrune = time.Now()
			}
		}
		cancel()
		if err != nil {
			r.fail("database_unavailable_or_schema_error")
			next = time.Now().Add(backoff)
			backoff = min(60*time.Second, backoff*2)
		} else {
			backoff = time.Second
			next = time.Time{}
			r.mu.Lock()
			now := time.Now().UTC()
			r.status.LastSuccess = &now
			r.status.Degraded = r.status.Dropped > 0 || r.status.QuarantinedFiles > 0
			r.status.Reason = ""
			if r.status.QuarantinedFiles > 0 {
				r.status.Reason = "corrupt_spool_quarantined"
			}
			if r.status.Dropped > 0 {
				r.status.Reason = "events_dropped"
			}
			r.mu.Unlock()
		}
	}
}
func (r *Recorder) discardQueue() {
	var n uint64
	for {
		select {
		case <-r.queue:
			n++
		default:
			r.mu.Lock()
			r.dropLocked("spool_unavailable", n)
			r.mu.Unlock()
			return
		}
	}
}
func (r *Recorder) deliver(ctx context.Context) error {
	if r.offsets == nil {
		r.offsets = map[string]int{}
		r.badPaths = map[string]bool{}
	}
	paths, _, err := r.files()
	if err != nil {
		return err
	}
	cutoff := time.Now().UTC().Add(-r.opt.Retention)
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		bad := r.badPaths[p]
		index := 0
		err = spoolLines(f, func(line []byte, oversized bool) (visitErr error) {
			index++
			if index <= r.offsets[p] {
				return nil
			}
			defer func() {
				if visitErr == nil {
					r.offsets[p] = index
				}
				if bad {
					r.badPaths[p] = true
				}
			}()
			if ctx.Err() != nil {
				return ctx.Err()
			}
			var e Event
			if oversized || json.Unmarshal(line, &e) != nil || !valid(e) {
				bad = true
				return nil
			}
			if r.opt.Retention > 0 && e.At.Before(cutoff) {
				if e.Payload.Broadcast == nil && e.Payload.Input == nil {
					return nil
				}
				// Use old lifecycle snapshots for recovery without retaining expired raw events.
				e.Expired = true
			}
			if err := r.backend.Write(ctx, e); err != nil {
				if invalidRow(err) {
					bad = true
					return nil
				}
				return err
			}
			return nil
		})
		if err != nil {
			f.Close()
			return err
		}
		f.Close()
		if bad {
			if err = os.Rename(p, strings.TrimSuffix(p, ".jsonl")+".bad"); err != nil {
				return err
			}
			r.mu.Lock()
			r.dropLocked("corrupt_spool_quarantined", 1)
			r.mu.Unlock()
			slog.Error("audit: corrupt spool quarantined")
		} else {
			if err = os.Remove(p); err != nil {
				return err
			}
		}
		delete(r.offsets, p)
		delete(r.badPaths, p)
		if err = syncDir(r.opt.Dir); err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	_, _, err = r.files()
	return err
}

// Bounded line reader: an oversized/corrupt line never hides later valid lines.
func spoolLines(src io.Reader, visit func([]byte, bool) error) error {
	r := bufio.NewReaderSize(src, 4096)
	var line []byte
	oversized := false
	for {
		part, err := r.ReadSlice('\n')
		if len(line)+len(part) > MaxEventBytes+1 {
			oversized = true
			line = nil
		}
		if !oversized {
			line = append(line, part...)
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		if len(line) > 0 || oversized {
			if e := visit(line, oversized); e != nil {
				return e
			}
		}
		line = nil
		oversized = false
		if errors.Is(err, io.EOF) {
			return nil
		}
	}
}
