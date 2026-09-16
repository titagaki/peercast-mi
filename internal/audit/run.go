package audit

import (
	"sync"
	"time"
)

// Run belongs to a single channel instance, never a reusable PCP channel ID.
// Its sink only enqueues bounded snapshots; no SQL or filesystem IO here.
type Run struct {
	mu        sync.Mutex
	sink      Sink
	b         Broadcast
	current   Settings
	inputs    map[string]*Input
	published map[string]time.Time
	committed bool
	pending   bool
	closed    bool
}

func NewRun(s Sink, channelID, owner string, a Actor, settings Settings, pending bool) *Run {
	if s == nil {
		return nil
	}
	a = canonical(Event{Actor: a}).Actor
	settings = settings.safe()
	r := &Run{sink: s, b: Broadcast{ID: ID(), ChannelID: channelID, Owner: Clip(owner, 255), OwnerName: a.Name, Actor: a, Settings: settings, Created: time.Now().UTC(), Status: "waiting"}, current: settings, inputs: map[string]*Input{}, published: map[string]time.Time{}, pending: pending}
	if owner != a.Account {
		r.b.OwnerName = ""
	}
	if !pending {
		r.Commit()
	}
	return r
}
func (r *Run) event(kind string, a Actor, reason, outcome string, in *Input, p Payload) {
	r.b.Revision++
	b := r.b
	p.Broadcast = &b
	if in != nil {
		v := *in
		p.Input = &v
	}
	e := Event{Type: kind, At: time.Now().UTC(), Actor: a, Outcome: outcome, Owner: b.Owner, BroadcastID: b.ID, ChannelID: b.ChannelID, Reason: reason, Payload: p}
	if in != nil {
		e.InputID = in.ID
		e.ConnectionID = in.ConnectionID
	}
	Send(r.sink, e)
}
func (r *Run) Commit() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.committed || r.closed {
		return
	}
	r.committed = true
	r.event("broadcast.create", r.b.Actor, "", "success", nil, Payload{})
}
func (r *Run) Snapshot() *Broadcast {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	b := r.b
	return &b
}
func (r *Run) Metadata(a Actor, s Settings) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	s = s.safe()
	s.InputGenre = r.current.InputGenre
	before := r.current
	if string(mustJSON(s)) == string(mustJSON(before)) || r.closed {
		return
	}
	r.current = s
	r.event("broadcast.metadata", a, "", "success", nil, Payload{Before: &before, After: &s})
}
func (r *Run) Media(connection, remote string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	now := time.Now().UTC()
	in := r.inputs[connection]
	kind := "input.progress"
	if in == nil {
		in = &Input{ID: ID(), BroadcastID: r.b.ID, ConnectionID: connection, RemoteIP: IP(remote), Started: now}
		r.inputs[connection] = in
		kind = "input.start"
	}
	in.LastMedia = now
	in.Revision++
	r.b.LastMedia = &now
	if r.b.FirstMedia == nil {
		r.b.FirstMedia = &now
	}
	r.b.Status = "live"
	if kind == "input.start" || now.Sub(r.published[connection]) >= time.Minute {
		r.event(kind, Actor{Source: "rtmp", IP: remote}, "", "success", in, Payload{})
		r.published[connection] = now
	}
}
func (r *Run) closeInput(connection, reason string, a Actor, now time.Time) {
	in := r.inputs[connection]
	if in == nil {
		return
	}
	in.Ended = &now
	in.Reason = reason
	in.Revision++
	r.event("input.end", a, reason, "success", in, Payload{})
	delete(r.inputs, connection)
	delete(r.published, connection)
}
func (r *Run) EndInput(connection, reason string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closeInput(connection, reason, Actor{Source: "rtmp"}, time.Now().UTC())
}
func (r *Run) End(a Actor, reason string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.closed = true
	now := time.Now().UTC()
	for conn := range r.inputs {
		r.closeInput(conn, reason, a, now)
	}
	r.b.Ended = &now
	r.b.Status = "ended"
	r.b.Reason = reason
	if r.pending && !r.committed {
		r.event("broadcast.create", r.b.Actor, reason, "failure", nil, Payload{})
	} else {
		r.event("broadcast.end", a, reason, "success", nil, Payload{})
	}
}
