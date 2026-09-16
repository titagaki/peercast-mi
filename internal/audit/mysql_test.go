package audit

import (
	"context"
	"database/sql"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

// Uses only the disposable, fixed-name test database. Never reads production credentials.
func testMariaDB(t *testing.T) *MySQL {
	t.Helper()
	addr := os.Getenv("PEERCAST_AUDIT_TEST_ADDR")
	if addr == "" {
		t.Skip("set PEERCAST_AUDIT_TEST_ADDR to a disposable MariaDB with database peercast_mi_audit_test")
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PEERCAST_AUDIT_DB_HOST", host)
	t.Setenv("PEERCAST_AUDIT_DB_PORT", port)
	t.Setenv("PEERCAST_AUDIT_DB_NAME", "peercast_mi_audit_test")
	t.Setenv("PEERCAST_AUDIT_DB_USER", "root")
	t.Setenv("PEERCAST_AUDIT_DB_PASSWORD", "")
	db, err := OpenMySQL()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.DB.Close() })
	return db
}

func TestMariaDB(t *testing.T) {
	var err error
	db := testMariaDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err = db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err = db.Migrate(ctx); err != nil {
		t.Fatal("repeat migration", err)
	}
	if err = db.Ready(ctx); err != nil {
		t.Fatal(err)
	}
	var version string
	if err = db.DB.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version); err != nil {
		t.Fatal(err)
	}
	t.Log("MariaDB", version)
	for _, table := range []string{"broadcast_inputs", "broadcasts", "audit_events"} {
		if _, err = db.DB.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.DB.ExecContext(ctx, "UPDATE schema_migrations SET checksum=REPEAT('0',64)"); err != nil {
		t.Fatal(err)
	}
	if db.Ready(ctx) == nil || db.Migrate(ctx) == nil {
		t.Fatal("checksum mismatch accepted")
	}
	if _, err = db.DB.ExecContext(ctx, "UPDATE schema_migrations SET checksum=?", Checksum()); err != nil {
		t.Fatal(err)
	}
	s := &capture{}
	owner := Actor{Account: "site:x:123", Name: "日本語🎮", IP: "2001:db8::42", Source: "site"}
	genre := "ゲーム"
	r := NewRun(s, ID(), owner.Account, owner, Settings{Name: "日本語🎮", InputGenre: &genre, Genre: "ypゲーム", Description: "詳細", ContactURL: "https://example.test/thread"}, false)
	r.Media(ID(), "[2001:db8::42]:1234")
	r.End(Actor{Account: "site:x:999", Source: "site"}, "admin_stop")
	boot := ID()
	stamp := func(e Event, n uint64) Event {
		e.ID = ID()
		e.Node = "test"
		e.Boot = boot
		e.Seq = n
		e.Version = 1
		return canonical(e)
	}
	for i := range s.events {
		s.events[i] = stamp(s.events[i], uint64(i+1))
	}
	// Newer snapshot arrives first; old snapshots must not reopen it.
	for i := len(s.events) - 1; i >= 0; i-- {
		if err = db.Write(ctx, s.events[i]); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range s.events {
		if err = db.Write(ctx, e); err != nil {
			t.Fatal("retry", err)
		}
	}
	var count int
	if err = db.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_events").Scan(&count); err != nil || count != len(s.events) {
		t.Fatal(count, err)
	}
	var status, name, ip string
	var ended sql.NullTime
	var revision uint64
	var incomplete bool
	if err = db.DB.QueryRowContext(ctx, "SELECT status,channel_name,create_ip,ended_at,revision,incomplete FROM broadcasts WHERE broadcast_id=?", r.Snapshot().ID).Scan(&status, &name, &ip, &ended, &revision, &incomplete); err != nil {
		t.Fatal(err)
	}
	if status != "ended" || name != "日本語🎮" || ip != "2001:db8::42" || !ended.Valid || revision != uint64(len(s.events)) || !incomplete {
		t.Fatal(status, name, ip, ended, revision, incomplete)
	}
	// Projection failure must roll back the raw event as well.
	invalid := stamp(s.events[0], 100)
	b := *invalid.Payload.Broadcast
	b.ID = ID()
	b.Settings.ContentType = strings.Repeat("x", 100)
	invalid.Payload.Broadcast = &b
	if err = db.Write(ctx, invalid); err == nil {
		t.Fatal("invalid projection accepted")
	}
	if err = db.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_events WHERE event_id=?", invalid.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("partial transaction", count, err)
	}
	// An old boot is interrupted without inventing an end; current boot survives.
	oldCapture := &capture{}
	old := NewRun(oldCapture, ID(), owner.Account, owner, Settings{Name: "old"}, false)
	old.Media(ID(), "192.0.2.10:123")
	for i, e := range oldCapture.events {
		e = stamp(e, uint64(200+i))
		if err = db.Write(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	newBoot := ID()
	current := fixtureEvent("broadcast.create")
	current.Boot = newBoot
	current.Payload.Broadcast = &Broadcast{ID: ID(), ChannelID: ID(), Actor: Actor{Source: "site"}, Settings: Settings{Name: "current"}, Created: time.Now().UTC(), Status: "waiting", Revision: 1}
	if err = db.Write(ctx, current); err != nil {
		t.Fatal(err)
	}
	recovery, err := db.Recover(ctx, "test", newBoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(recovery) != 2 {
		t.Fatal(recovery)
	}
	for i, e := range recovery {
		e = stamp(e, uint64(300+i))
		e.Boot = newBoot
		if err = db.Write(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	if err = db.DB.QueryRowContext(ctx, "SELECT status,ended_at,incomplete FROM broadcasts WHERE broadcast_id=?", old.Snapshot().ID).Scan(&status, &ended, &incomplete); err != nil {
		t.Fatal(err)
	}
	if status != "interrupted" || ended.Valid || !incomplete {
		t.Fatal(status, ended, incomplete)
	}
	if again, err := db.Recover(ctx, "test", newBoot); err != nil || len(again) != 0 {
		t.Fatal(again, err)
	}
	// Expired lifecycle snapshots reconcile without restoring expired raw records.
	expired := stamp(s.events[len(s.events)-1], 400)
	expired.Expired = true
	b = *expired.Payload.Broadcast
	b.ID = ID()
	past := time.Now().Add(-100 * 24 * time.Hour)
	b.Ended = &past
	expired.Payload.Broadcast = &b
	if err = db.Write(ctx, expired); err != nil {
		t.Fatal(err)
	}
	if err = db.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_events WHERE event_id=?", expired.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	if err = db.Prune(ctx, time.Now().Add(-90*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err = db.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM broadcasts WHERE broadcast_id=?", b.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("expired projection retained", count, err)
	}
	if err = db.DB.QueryRowContext(ctx, "SELECT status FROM broadcasts WHERE broadcast_id=?", current.Payload.Broadcast.ID).Scan(&status); err != nil || status != "waiting" {
		t.Fatal("current boot closed", status, err)
	}
}
