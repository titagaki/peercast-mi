package audit

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestParseFilter(t *testing.T) {
	for _, tc := range []struct {
		kind, query string
		valid       bool
	}{
		{"events", "", true}, {"events", "limit=101", false}, {"events", "limit=-1", false}, {"events", "limit=1&limit=2", false},
		{"events", "from=2026-09-17", false}, {"events", "from=2026-09-17T01:00:00Z&until=2026-09-17T00:00:00Z", false},
		{"events", "ip=127.0.0.1", false}, {"events", "from=2026-09-01T00:00:00Z&until=2026-09-30T00:00:00Z&ip=2001:db8::1", true},
		{"events", "from=2026-01-01T00:00:00Z&until=2026-09-30T00:00:00Z&ip=2001:db8::1", false},
		{"events", "cursor=bad", false}, {"events", "broadcastId=invalid", false}, {"events", "sql=SELECT", false},
		{"broadcasts", "actor=site:x:1", false}, {"inputs:" + strings.Repeat("a", 32), "type=auth.login", false},
	} {
		values, err := url.ParseQuery(tc.query)
		if err != nil {
			t.Fatal(err)
		}
		_, err = ParseFilter(tc.kind, values)
		if (err == nil) != tc.valid {
			t.Fatalf("%s %s: %v", tc.kind, tc.query, err)
		}
	}
	raw, _ := json.Marshal(position{Kind: "events", ID: ID(), At: time.Now()})
	token := base64.RawURLEncoding.EncodeToString(raw)
	if _, err := ParseFilter("broadcasts", url.Values{"cursor": {token}}); err == nil {
		t.Fatal("cursor crossed collection")
	}
	f, err := ParseFilter("events", url.Values{"from": {"2026-09-17T09:00:00+09:00"}, "until": {"2026-09-18T09:00:00+09:00"}, "ip": {"::ffff:192.0.2.1"}})
	if err != nil || f.From.Hour() != 0 || f.IP != "192.0.2.1" || f.Limit != 50 {
		t.Fatal(f, err)
	}
}
func TestMariaDBReader(t *testing.T) {
	db := testMariaDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	owner := "reader:" + ID()
	boot := ID()
	at := time.Date(2026, 9, 16, 14, 0, 0, 456000, time.UTC)
	ids := []string{}
	for i := 0; i < 5; i++ {
		e := fixtureEvent("auth.login")
		e.Boot = boot
		e.Seq = uint64(i + 1)
		e.ID = boot[:16] + fmt.Sprintf("%016x", i)
		e.At = at
		e.Actor = Actor{Account: owner, Name: "日本語🎮", IP: "2001:db8::1", Source: "site"}
		if i > 2 {
			e.At = at.Add(time.Duration(i-2) * time.Hour)
		}
		if err := db.Write(ctx, e); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, e.ID)
	}
	values := url.Values{"actor": {owner}, "from": {at.Format(time.RFC3339Nano)}, "until": {at.Add(2 * time.Hour).Format(time.RFC3339Nano)}, "ip": {"2001:db8::1"}, "type": {"auth.login"}, "outcome": {"success"}, "limit": {"2"}}
	filter, _ := ParseFilter("events", values)
	page, err := db.Events(ctx, filter)
	if err != nil || len(page.Items) != 2 || page.Items[0].ID != ids[3] || page.Items[1].ID != ids[2] || page.NextCursor == "" {
		t.Fatal(page, err)
	}
	values.Set("cursor", page.NextCursor)
	filter, err = ParseFilter("events", values)
	if err != nil {
		t.Fatal(err)
	}
	page, err = db.Events(ctx, filter)
	if err != nil || len(page.Items) != 2 || page.Items[0].ID != ids[1] || page.Items[1].ID != ids[0] || page.NextCursor != "" {
		t.Fatal(page, err)
	}
	if page.Items[0].Actor.Name != "日本語🎮" || page.Items[0].RecordedAt.IsZero() {
		t.Fatal("event fields missing")
	}
	filter, _ = ParseFilter("events", url.Values{"actor": {"' OR 1=1 --"}})
	page, err = db.Events(ctx, filter)
	if err != nil || len(page.Items) != 0 {
		t.Fatal("unbound filter", page, err)
	}
	// Unexpected JSON fields in the DB are not exposed as API payload fields.
	if _, err = db.DB.ExecContext(ctx, `UPDATE audit_events SET payload='{"oauthToken":"must-not-escape"}' WHERE event_id=?`, ids[0]); err != nil {
		t.Fatal(err)
	}
	filter, _ = ParseFilter("events", url.Values{"actor": {owner}})
	page, err = db.Events(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(page)
	if strings.Contains(string(raw), "must-not-escape") {
		t.Fatal("unknown DB payload leaked")
	}

	runIDs := []string{ID(), ID()}
	channelID := ID()
	for i, runID := range runIDs {
		e := fixtureEvent("broadcast.create")
		e.Boot = boot
		e.Seq = uint64(100 + i)
		e.Owner = owner
		e.BroadcastID = runID
		e.ChannelID = channelID
		e.Payload.Broadcast = &Broadcast{ID: runID, ChannelID: channelID, Owner: owner, OwnerName: "配信者", Actor: Actor{Account: owner, IP: "192.0.2.1", Source: "site"}, Settings: Settings{Name: "いまいch", Genre: "ypゲーム", Description: "内容"}, Created: at.Add(time.Duration(i) * time.Hour), Status: "interrupted", Interrupted: &at, Incomplete: true, Revision: 1}
		if err = db.Write(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	filter, _ = ParseFilter("broadcasts", url.Values{"owner": {owner}, "status": {"interrupted"}, "channelId": {channelID}, "limit": {"1"}})
	bp, err := db.Broadcasts(ctx, filter)
	if err != nil || len(bp.Items) != 1 || bp.Items[0].ID != runIDs[1] || bp.Items[0].Ended != nil || bp.NextCursor == "" {
		t.Fatal(bp, err)
	}
	filter, _ = ParseFilter("broadcasts", url.Values{"owner": {owner}, "cursor": {bp.NextCursor}, "limit": {"1"}})
	bp, err = db.Broadcasts(ctx, filter)
	if err != nil || len(bp.Items) != 1 || bp.Items[0].ID != runIDs[0] || bp.NextCursor != "" {
		t.Fatal(bp, err)
	}
	for i := 0; i < 3; i++ {
		e := fixtureEvent("input.start")
		e.Boot = boot
		e.Seq = uint64(200 + i)
		runID := runIDs[0]
		if i == 2 {
			runID = runIDs[1]
		}
		e.Payload.Input = &Input{ID: ID(), BroadcastID: runID, ConnectionID: ID(), RemoteIP: "2001:db8::42", Started: at.Add(time.Duration(i) * time.Minute), LastMedia: at.Add(time.Duration(i) * time.Minute), Revision: 1}
		if err = db.Write(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	filter, _ = ParseFilter("inputs:"+runIDs[0], url.Values{"limit": {"1"}})
	ip, err := db.Inputs(ctx, runIDs[0], filter)
	if err != nil || len(ip.Items) != 1 || ip.Items[0].BroadcastID != runIDs[0] || ip.NextCursor == "" {
		t.Fatal(ip, err)
	}
	if _, err = ParseFilter("inputs:"+runIDs[1], url.Values{"cursor": {ip.NextCursor}}); err == nil {
		t.Fatal("input cursor crossed broadcast")
	}
	firstID := ip.Items[0].ID
	filter, _ = ParseFilter("inputs:"+runIDs[0], url.Values{"limit": {"1"}, "cursor": {ip.NextCursor}})
	ip, err = db.Inputs(ctx, runIDs[0], filter)
	if err != nil || len(ip.Items) != 1 || ip.Items[0].ID == firstID || ip.NextCursor != "" {
		t.Fatal(ip, err)
	}
}
