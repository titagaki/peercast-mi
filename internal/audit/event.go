// Package audit records operational history without blocking media on database IO.
package audit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/netip"
	"strings"
	"time"
)

func ID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func IP(remote string) string {
	h, _, err := net.SplitHostPort(remote)
	if err != nil {
		h = remote
	}
	ip, err := netip.ParseAddr(h)
	if err != nil {
		return ""
	}
	return ip.Unmap().WithZone("").String()
}

// Clip limits untrusted text on UTF-8 boundaries before persistence.
func Clip(s string, n int) string {
	if len(s) <= n {
		return strings.ToValidUTF8(s, "")
	}
	s = s[:n]
	return strings.ToValidUTF8(s, "")
}

type Actor struct {
	Account string `json:"account,omitempty"`
	Name    string `json:"name,omitempty"`
	IP      string `json:"ip,omitempty"`
	Session string `json:"session,omitempty"`
	Source  string `json:"source"`
}
type Settings struct {
	Name        string  `json:"name"`
	InputGenre  *string `json:"inputGenre,omitempty"`
	Genre       string  `json:"genre"`
	Description string  `json:"description"`
	Comment     string  `json:"comment"`
	ContactURL  string  `json:"contactUrl"`
	Bitrate     uint32  `json:"bitrate"`
	ContentType string  `json:"contentType"`
}

func (s Settings) safe() Settings {
	s.Name = Clip(s.Name, 256)
	s.Genre = Clip(s.Genre, 256)
	s.Description = Clip(s.Description, 2048)
	s.Comment = Clip(s.Comment, 2048)
	s.ContactURL = Clip(s.ContactURL, 2048)
	s.ContentType = Clip(s.ContentType, 32)
	if s.InputGenre != nil {
		v := Clip(*s.InputGenre, 256)
		s.InputGenre = &v
	}
	return s
}

type Broadcast struct {
	Node        string     `json:"node,omitempty"`
	Boot        string     `json:"boot,omitempty"`
	ID          string     `json:"id"`
	ChannelID   string     `json:"channelId"`
	Owner       string     `json:"owner,omitempty"`
	OwnerName   string     `json:"ownerName,omitempty"`
	Actor       Actor      `json:"actor"`
	Settings    Settings   `json:"settings"`
	Created     time.Time  `json:"created"`
	FirstMedia  *time.Time `json:"firstMedia,omitempty"`
	LastMedia   *time.Time `json:"lastMedia,omitempty"`
	Ended       *time.Time `json:"ended,omitempty"`
	Interrupted *time.Time `json:"interrupted,omitempty"`
	Status      string     `json:"status"`
	Reason      string     `json:"reason,omitempty"`
	Incomplete  bool       `json:"incomplete"`
	Revision    uint64     `json:"revision"`
}
type Input struct {
	ID           string     `json:"id"`
	BroadcastID  string     `json:"broadcastId"`
	ConnectionID string     `json:"connectionId"`
	RemoteIP     string     `json:"remoteIp"`
	Started      time.Time  `json:"started"`
	LastMedia    time.Time  `json:"lastMedia"`
	Ended        *time.Time `json:"ended,omitempty"`
	Interrupted  *time.Time `json:"interrupted,omitempty"`
	Reason       string     `json:"reason,omitempty"`
	Incomplete   bool       `json:"incomplete"`
	Revision     uint64     `json:"revision"`
}
type Payload struct {
	Broadcast *Broadcast `json:"broadcast,omitempty"`
	Input     *Input     `json:"input,omitempty"`
	Before    *Settings  `json:"before,omitempty"`
	After     *Settings  `json:"after,omitempty"`
	Count     uint64     `json:"count,omitempty"`
	FirstAt   *time.Time `json:"firstAt,omitempty"`
	LastAt    *time.Time `json:"lastAt,omitempty"`
}
type Event struct {
	Expired      bool      `json:"-"`
	ID           string    `json:"id"`
	Node         string    `json:"node"`
	Boot         string    `json:"boot"`
	Seq          uint64    `json:"seq"`
	At           time.Time `json:"at"`
	Type         string    `json:"type"`
	Actor        Actor     `json:"actor"`
	Outcome      string    `json:"outcome"`
	Owner        string    `json:"owner,omitempty"`
	BroadcastID  string    `json:"broadcastId,omitempty"`
	InputID      string    `json:"inputId,omitempty"`
	ConnectionID string    `json:"connectionId,omitempty"`
	ChannelID    string    `json:"channelId,omitempty"`
	Reason       string    `json:"reason,omitempty"`
	Version      int       `json:"version"`
	Payload      Payload   `json:"payload"`
}
type Sink interface{ Emit(Event) }

func Send(s Sink, e Event) {
	if s != nil {
		s.Emit(e)
	}
}
func canonical(e Event) Event {
	e.Actor.Account = Clip(e.Actor.Account, 255)
	e.Actor.Name = Clip(e.Actor.Name, 256)
	e.Actor.IP = IP(e.Actor.IP)
	e.Owner = Clip(e.Owner, 255)
	if e.Actor.Source == "" {
		e.Actor.Source = "system"
	}
	return e
}
func valid(e Event) bool {
	hexID := func(s string) bool {
		if len(s) != 32 {
			return false
		}
		_, err := hex.DecodeString(s)
		return err == nil
	}
	ascii := func(s string, n int) bool {
		if len(s) > n {
			return false
		}
		for _, c := range s {
			if c < 32 || c > 126 {
				return false
			}
		}
		return true
	}
	if !hexID(e.ID) || !hexID(e.Boot) || e.Node == "" || !ascii(e.Node, 64) || e.Version != 1 || e.Seq == 0 || e.At.IsZero() {
		return false
	}
	if !ascii(e.Type, 48) || !ascii(e.Actor.Source, 16) || !ascii(e.Reason, 64) || !ascii(e.Outcome, 16) {
		return false
	}
	for _, id := range []string{e.Actor.Session, e.BroadcastID, e.InputID, e.ConnectionID, e.ChannelID} {
		if id != "" && !hexID(id) {
			return false
		}
	}
	switch e.Type {
	case "auth.login", "auth.logout", "key.issue", "key.rotate", "key.revoke", "broadcast.create", "broadcast.metadata", "broadcast.end", "broadcast.interrupted", "rtmp.publish", "input.start", "input.progress", "input.end", "system.start", "system.stop", "audit.gap":
	default:
		return false
	}
	if b := e.Payload.Broadcast; b != nil {
		if !hexID(b.ID) || !hexID(b.ChannelID) || b.Revision == 0 || b.Created.IsZero() {
			return false
		}
	}
	if in := e.Payload.Input; in != nil {
		if !hexID(in.ID) || !hexID(in.BroadcastID) || !hexID(in.ConnectionID) || in.Revision == 0 || in.Started.IsZero() {
			return false
		}
	}
	return true
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

// Context actors are installed only by the in-process, authenticated site route.
// No HTTP header is accepted as an identity assertion.
type actorKey struct{}

func WithActor(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, actorKey{}, a)
}
func ContextActor(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(actorKey{}).(Actor)
	return a, ok
}
