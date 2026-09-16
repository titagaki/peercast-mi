package audit

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Reader uses a separate connection pool from the event writer in production.
// All columns and sort keys are fixed here; user input is bound as SQL values.
type Reader interface {
	Events(context.Context, Filter) (Page[EventRecord], error)
	Broadcasts(context.Context, Filter) (Page[Broadcast], error)
	Inputs(context.Context, string, Filter) (Page[Input], error)
}
type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"nextCursor,omitempty"`
}
type EventRecord struct {
	Event
	RecordedAt time.Time `json:"recordedAt"`
}
type position struct {
	Kind string    `json:"kind"`
	At   time.Time `json:"at"`
	ID   string    `json:"id"`
}
type Filter struct {
	From, Until                                                     time.Time
	Actor, Owner, Type, Outcome, Status, IP, BroadcastID, ChannelID string
	Limit                                                           int
	before                                                          *position
}

func HexID(s string) bool {
	if len(s) != 32 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// ParseFilter validates the endpoint-specific query before touching the DB.
func ParseFilter(kind string, values url.Values) (Filter, error) {
	f := Filter{Limit: 50}
	allowed := map[string]bool{"limit": true, "cursor": true}
	switch kind {
	case "events":
		for _, k := range []string{"from", "until", "actor", "owner", "type", "outcome", "ip", "broadcastId", "channelId"} {
			allowed[k] = true
		}
	case "broadcasts":
		for _, k := range []string{"from", "until", "owner", "status", "channelId"} {
			allowed[k] = true
		}
	default:
		if !strings.HasPrefix(kind, "inputs:") || !HexID(strings.TrimPrefix(kind, "inputs:")) {
			return f, errors.New("不正な検索対象です。")
		}
	}
	for k, v := range values {
		if !allowed[k] || len(v) != 1 || len(v[0]) > 1024 {
			return f, errors.New("検索条件が不正です。")
		}
	}
	if v := values.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 100 {
			return f, errors.New("取得件数は1〜100件です。")
		}
		f.Limit = n
	}
	for k, target := range map[string]*time.Time{"from": &f.From, "until": &f.Until} {
		if v := values.Get(k); v != "" {
			t, err := time.Parse(time.RFC3339Nano, v)
			if err != nil || t.Year() < 1000 || t.Year() > 9999 {
				return f, errors.New("日時が不正です。")
			}
			*target = t.UTC()
		}
	}
	if !f.From.IsZero() && !f.Until.IsZero() && !f.From.Before(f.Until) {
		return f, errors.New("終了日時は開始日時より後にしてください。")
	}
	f.Actor = values.Get("actor")
	f.Owner = values.Get("owner")
	f.Type = values.Get("type")
	f.Outcome = values.Get("outcome")
	f.Status = values.Get("status")
	for _, v := range []string{f.Actor, f.Owner} {
		if len(v) > 255 {
			return f, errors.New("アカウントが長すぎます。")
		}
	}
	if len(f.Type) > 48 || len(f.Outcome) > 16 || len(f.Status) > 16 {
		return f, errors.New("検索条件が長すぎます。")
	}
	f.BroadcastID = strings.ToLower(values.Get("broadcastId"))
	f.ChannelID = strings.ToLower(values.Get("channelId"))
	for _, v := range []string{f.BroadcastID, f.ChannelID} {
		if v != "" && !HexID(v) {
			return f, errors.New("IDは32桁の16進数で指定してください。")
		}
	}
	if v := values.Get("ip"); v != "" {
		ip, err := netip.ParseAddr(v)
		if err != nil || ip.Zone() != "" {
			return f, errors.New("IPアドレスが不正です。")
		}
		f.IP = ip.Unmap().String()
		if f.From.IsZero() || f.Until.IsZero() || f.Until.Sub(f.From) > 31*24*time.Hour {
			return f, errors.New("IP検索では31日以内の開始・終了日時を指定してください。")
		}
	}
	if token := values.Get("cursor"); token != "" {
		raw, err := base64.RawURLEncoding.DecodeString(token)
		if err != nil {
			return f, errors.New("ページ指定が不正です。")
		}
		var p position
		if json.Unmarshal(raw, &p) != nil || p.Kind != kind || !HexID(p.ID) || p.At.IsZero() || p.At.Year() < 1000 || p.At.Year() > 9999 {
			return f, errors.New("ページ指定が不正です。")
		}
		p.At = p.At.UTC()
		f.before = &p
	}
	return f, nil
}
func pageOf[T any](kind string, items []T, f Filter, key func(T) (time.Time, string)) Page[T] {
	page := Page[T]{Items: items}
	if len(items) > f.Limit {
		page.Items = items[:f.Limit]
		at, id := key(page.Items[len(page.Items)-1])
		raw, _ := json.Marshal(position{Kind: kind, At: at, ID: id})
		page.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	return page
}

type selection struct {
	clauses []string
	args    []any
}

func (s *selection) equal(column, value string) {
	if value != "" {
		s.clauses = append(s.clauses, column+"=?")
		s.args = append(s.args, value)
	}
}
func (s *selection) tail(f Filter, dateColumn, idColumn string) string {
	if !f.From.IsZero() {
		s.clauses = append(s.clauses, dateColumn+">=?")
		s.args = append(s.args, f.From)
	}
	if !f.Until.IsZero() {
		s.clauses = append(s.clauses, dateColumn+"<?")
		s.args = append(s.args, f.Until)
	}
	if f.before != nil {
		s.clauses = append(s.clauses, "("+dateColumn+"<? OR ("+dateColumn+"=? AND "+idColumn+"<?))")
		s.args = append(s.args, f.before.At, f.before.At, f.before.ID)
	}
	where := ""
	if len(s.clauses) > 0 {
		where = " WHERE " + strings.Join(s.clauses, " AND ")
	}
	s.args = append(s.args, f.Limit+1)
	return where + " ORDER BY " + dateColumn + " DESC," + idColumn + " DESC LIMIT ?"
}
func (m *MySQL) Events(ctx context.Context, f Filter) (Page[EventRecord], error) {
	if f.Limit < 1 || f.Limit > 100 {
		return Page[EventRecord]{}, errors.New("invalid page limit")
	}
	out := make([]EventRecord, 0)
	if err := m.Ready(ctx); err != nil {
		return Page[EventRecord]{}, err
	}
	s := selection{}
	s.equal("actor_account", f.Actor)
	s.equal("owner_account", f.Owner)
	s.equal("event_type", f.Type)
	s.equal("outcome", f.Outcome)
	s.equal("client_ip", f.IP)
	s.equal("broadcast_id", f.BroadcastID)
	s.equal("channel_id", f.ChannelID)
	query := `SELECT event_id,node_id,boot_id,boot_seq,occurred_at,recorded_at,event_type,source,outcome,COALESCE(actor_account,''),COALESCE(actor_name,''),COALESCE(owner_account,''),COALESCE(client_ip,''),COALESCE(session_ref,''),COALESCE(broadcast_id,''),COALESCE(input_id,''),COALESCE(connection_id,''),COALESCE(channel_id,''),COALESCE(reason_code,''),payload_version,payload FROM audit_events` + s.tail(f, "occurred_at", "event_id")
	rows, err := m.DB.QueryContext(ctx, query, s.args...)
	if err != nil {
		return Page[EventRecord]{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var e EventRecord
		var payload []byte
		if err = rows.Scan(&e.ID, &e.Node, &e.Boot, &e.Seq, &e.At, &e.RecordedAt, &e.Type, &e.Actor.Source, &e.Outcome, &e.Actor.Account, &e.Actor.Name, &e.Owner, &e.Actor.IP, &e.Actor.Session, &e.BroadcastID, &e.InputID, &e.ConnectionID, &e.ChannelID, &e.Reason, &e.Version, &payload); err != nil {
			return Page[EventRecord]{}, err
		}
		// Decode the allowlisted payload, not arbitrary DB JSON into the response.
		if err = json.Unmarshal(payload, &e.Payload); err != nil {
			return Page[EventRecord]{}, err
		}
		out = append(out, e)
	}
	return pageOf("events", out, f, func(e EventRecord) (time.Time, string) { return e.At, e.ID }), rows.Err()
}
func (m *MySQL) Broadcasts(ctx context.Context, f Filter) (Page[Broadcast], error) {
	if f.Limit < 1 || f.Limit > 100 {
		return Page[Broadcast]{}, errors.New("invalid page limit")
	}
	out := make([]Broadcast, 0)
	if err := m.Ready(ctx); err != nil {
		return Page[Broadcast]{}, err
	}
	s := selection{}
	s.equal("owner_account", f.Owner)
	s.equal("status", f.Status)
	s.equal("channel_id", f.ChannelID)
	query := `SELECT broadcast_id,node_id,boot_id,channel_id,COALESCE(owner_account,''),COALESCE(owner_name,''),COALESCE(created_by,''),source,COALESCE(create_ip,''),channel_name,input_genre,genre,description,comment,contact_url,bitrate,content_type,created_at,first_media_at,last_media_at,ended_at,interruption_detected_at,status,COALESCE(end_reason,''),incomplete,revision FROM broadcasts` + s.tail(f, "created_at", "broadcast_id")
	rows, err := m.DB.QueryContext(ctx, query, s.args...)
	if err != nil {
		return Page[Broadcast]{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var b Broadcast
		if err = rows.Scan(&b.ID, &b.Node, &b.Boot, &b.ChannelID, &b.Owner, &b.OwnerName, &b.Actor.Account, &b.Actor.Source, &b.Actor.IP, &b.Settings.Name, &b.Settings.InputGenre, &b.Settings.Genre, &b.Settings.Description, &b.Settings.Comment, &b.Settings.ContactURL, &b.Settings.Bitrate, &b.Settings.ContentType, &b.Created, &b.FirstMedia, &b.LastMedia, &b.Ended, &b.Interrupted, &b.Status, &b.Reason, &b.Incomplete, &b.Revision); err != nil {
			return Page[Broadcast]{}, err
		}
		out = append(out, b)
	}
	return pageOf("broadcasts", out, f, func(b Broadcast) (time.Time, string) { return b.Created, b.ID }), rows.Err()
}
func (m *MySQL) Inputs(ctx context.Context, id string, f Filter) (Page[Input], error) {
	if !HexID(id) || f.Limit < 1 || f.Limit > 100 {
		return Page[Input]{}, errors.New("invalid input query")
	}
	out := make([]Input, 0)
	if err := m.Ready(ctx); err != nil {
		return Page[Input]{}, err
	}
	s := selection{}
	s.equal("broadcast_id", id)
	query := `SELECT input_id,broadcast_id,connection_id,remote_ip,started_at,last_media_at,ended_at,interruption_detected_at,COALESCE(end_reason,''),incomplete,revision FROM broadcast_inputs` + s.tail(f, "started_at", "input_id")
	rows, err := m.DB.QueryContext(ctx, query, s.args...)
	if err != nil {
		return Page[Input]{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var in Input
		if err = rows.Scan(&in.ID, &in.BroadcastID, &in.ConnectionID, &in.RemoteIP, &in.Started, &in.LastMedia, &in.Ended, &in.Interrupted, &in.Reason, &in.Incomplete, &in.Revision); err != nil {
			return Page[Input]{}, err
		}
		out = append(out, in)
	}
	return pageOf("inputs:"+id, out, f, func(in Input) (time.Time, string) { return in.Started, in.ID }), rows.Err()
}
