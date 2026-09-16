package audit

import (
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

//go:embed migrations/001_initial.sql
var Schema string

func Checksum() string { b := sha256.Sum256([]byte(Schema)); return hex.EncodeToString(b[:]) }

type MySQL struct{ DB *sql.DB }

func OpenMySQL() (*MySQL, error) {
	cfg := mysql.NewConfig()
	cfg.User = os.Getenv("PEERCAST_AUDIT_DB_USER")
	cfg.Passwd = os.Getenv("PEERCAST_AUDIT_DB_PASSWORD")
	cfg.DBName = os.Getenv("PEERCAST_AUDIT_DB_NAME")
	host := os.Getenv("PEERCAST_AUDIT_DB_HOST")
	port := os.Getenv("PEERCAST_AUDIT_DB_PORT")
	if port == "" {
		port = "3306"
	}
	if cfg.User == "" || cfg.DBName == "" || host == "" {
		return nil, errors.New("audit database environment is incomplete")
	}
	cfg.Net = "tcp"
	cfg.Addr = net.JoinHostPort(host, port)
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	cfg.Params = map[string]string{"time_zone": "'+00:00'", "charset": "utf8mb4"}
	cfg.Timeout = 2 * time.Second
	cfg.ReadTimeout = 3 * time.Second
	cfg.WriteTimeout = 3 * time.Second
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, errors.New("invalid audit database configuration")
	}
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)
	return &MySQL{db}, nil
}
func (m *MySQL) Ready(ctx context.Context) error {
	if m == nil || m.DB == nil {
		return errors.New("audit database not configured")
	}
	var checksum string
	err := m.DB.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE version=1").Scan(&checksum)
	if err != nil {
		return err
	}
	if checksum != Checksum() {
		return errors.New("audit migration checksum mismatch")
	}
	return nil
}
func nullable(v string) any {
	if v == "" {
		return nil
	}
	return v
}
func (m *MySQL) Write(ctx context.Context, e Event) error {
	tx, err := m.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if !e.Expired {
		// A duplicate is accepted only if this exact event is already committed.
		var id string
		err = tx.QueryRowContext(ctx, "SELECT event_id FROM audit_events WHERE event_id=?", e.ID).Scan(&id)
		if err == nil {
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO audit_events (event_id,node_id,boot_id,boot_seq,occurred_at,event_type,source,outcome,actor_account,actor_name,owner_account,client_ip,session_ref,broadcast_id,input_id,connection_id,channel_id,reason_code,payload_version,payload) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, e.ID, e.Node, e.Boot, e.Seq, e.At, e.Type, e.Actor.Source, e.Outcome, nullable(e.Actor.Account), nullable(e.Actor.Name), nullable(e.Owner), nullable(e.Actor.IP), nullable(e.Actor.Session), nullable(e.BroadcastID), nullable(e.InputID), nullable(e.ConnectionID), nullable(e.ChannelID), nullable(e.Reason), e.Version, string(mustJSON(e.Payload)))
		if err != nil {
			return err
		}
	}
	if b := e.Payload.Broadcast; b != nil {
		node, boot := b.Node, b.Boot
		if node == "" {
			node = e.Node
		}
		if boot == "" {
			boot = e.Boot
		}
		snapshot := *b
		b = &snapshot
		if e.Type != "broadcast.create" {
			var exists int
			if er := tx.QueryRowContext(ctx, "SELECT 1 FROM broadcasts WHERE broadcast_id=?", b.ID).Scan(&exists); errors.Is(er, sql.ErrNoRows) {
				b.Incomplete = true
			} else if er != nil {
				return er
			}
		}
		cols := []string{"broadcast_id", "node_id", "boot_id", "channel_id", "owner_account", "owner_name", "created_by", "source", "create_ip", "channel_name", "input_genre", "genre", "description", "comment", "contact_url", "bitrate", "content_type", "created_at", "first_media_at", "last_media_at", "ended_at", "interruption_detected_at", "status", "end_reason", "incomplete", "source_event_id", "revision"}
		vals := []any{b.ID, node, boot, b.ChannelID, nullable(b.Owner), nullable(b.OwnerName), nullable(b.Actor.Account), b.Actor.Source, nullable(IP(b.Actor.IP)), b.Settings.Name, b.Settings.InputGenre, b.Settings.Genre, b.Settings.Description, b.Settings.Comment, b.Settings.ContactURL, b.Settings.Bitrate, b.Settings.ContentType, b.Created, b.FirstMedia, b.LastMedia, b.Ended, b.Interrupted, b.Status, nullable(b.Reason), b.Incomplete, e.ID, b.Revision}
		if err = upsert(ctx, tx, "broadcasts", cols, vals); err != nil {
			return err
		}
	}
	if in := e.Payload.Input; in != nil {
		snapshot := *in
		in = &snapshot
		if e.Type != "input.start" {
			var exists int
			if er := tx.QueryRowContext(ctx, "SELECT 1 FROM broadcast_inputs WHERE input_id=?", in.ID).Scan(&exists); errors.Is(er, sql.ErrNoRows) {
				in.Incomplete = true
			} else if er != nil {
				return er
			}
		}
		cols := []string{"input_id", "broadcast_id", "connection_id", "remote_ip", "started_at", "last_media_at", "ended_at", "interruption_detected_at", "end_reason", "incomplete", "source_event_id", "revision"}
		vals := []any{in.ID, in.BroadcastID, in.ConnectionID, in.RemoteIP, in.Started, in.LastMedia, in.Ended, in.Interrupted, nullable(in.Reason), in.Incomplete, e.ID, in.Revision}
		if err = upsert(ctx, tx, "broadcast_inputs", cols, vals); err != nil {
			return err
		}
	}
	if e.Type == "audit.gap" {
		if _, err = tx.ExecContext(ctx, "UPDATE broadcasts SET incomplete=TRUE WHERE node_id=? AND boot_id=?", e.Node, e.Boot); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE broadcast_inputs i JOIN broadcasts b USING(broadcast_id) SET i.incomplete=TRUE WHERE b.node_id=? AND b.boot_id=?", e.Node, e.Boot); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func upsert(ctx context.Context, tx *sql.Tx, table string, cols []string, values []any) error {
	updates := []string{}
	for _, c := range cols[1 : len(cols)-1] {
		if c == "incomplete" {
			updates = append(updates, "incomplete=(incomplete OR VALUES(incomplete))")
			continue
		}
		updates = append(updates, fmt.Sprintf("%s=IF(revision<VALUES(revision),VALUES(%s),%s)", c, c, c))
	}
	updates = append(updates, "updated_at=IF(revision<VALUES(revision),UTC_TIMESTAMP(6),updated_at)", "revision=GREATEST(revision,VALUES(revision))")
	_, err := tx.ExecContext(ctx, "INSERT INTO "+table+" ("+strings.Join(cols, ",")+") VALUES ("+strings.TrimSuffix(strings.Repeat("?,", len(cols)), ",")+") ON DUPLICATE KEY UPDATE "+strings.Join(updates, ","), values...)
	return err
}

func (m *MySQL) Recover(ctx context.Context, node, boot string) ([]Event, error) {
	rows, err := m.DB.QueryContext(ctx, `SELECT broadcast_id,node_id,boot_id,channel_id,COALESCE(owner_account,''),COALESCE(owner_name,''),COALESCE(created_by,''),source,COALESCE(create_ip,''),channel_name,input_genre,genre,description,comment,contact_url,bitrate,content_type,created_at,first_media_at,last_media_at,revision FROM broadcasts WHERE node_id=? AND boot_id<>? AND status IN ('waiting','live') LIMIT 50`, node, boot)
	if err != nil {
		return nil, err
	}
	var bs []Broadcast
	for rows.Next() {
		var b Broadcast
		err = rows.Scan(&b.ID, &b.Node, &b.Boot, &b.ChannelID, &b.Owner, &b.OwnerName, &b.Actor.Account, &b.Actor.Source, &b.Actor.IP, &b.Settings.Name, &b.Settings.InputGenre, &b.Settings.Genre, &b.Settings.Description, &b.Settings.Comment, &b.Settings.ContactURL, &b.Settings.Bitrate, &b.Settings.ContentType, &b.Created, &b.FirstMedia, &b.LastMedia, &b.Revision)
		if err != nil {
			rows.Close()
			return nil, err
		}
		bs = append(bs, b)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	var events []Event
	now := time.Now().UTC()
	for _, b := range bs {
		ir, err := m.DB.QueryContext(ctx, `SELECT input_id,connection_id,remote_ip,started_at,last_media_at,revision FROM broadcast_inputs WHERE broadcast_id=? AND ended_at IS NULL AND interruption_detected_at IS NULL`, b.ID)
		if err != nil {
			return nil, err
		}
		for ir.Next() {
			var in Input
			in.BroadcastID = b.ID
			if err = ir.Scan(&in.ID, &in.ConnectionID, &in.RemoteIP, &in.Started, &in.LastMedia, &in.Revision); err != nil {
				ir.Close()
				return nil, err
			}
			in.Revision++
			in.Incomplete = true
			in.Interrupted = &now
			in.Reason = "process_interrupted"
			events = append(events, Event{Type: "input.end", At: now, Actor: Actor{Source: "system"}, Outcome: "unknown", Reason: in.Reason, Owner: b.Owner, BroadcastID: b.ID, ChannelID: b.ChannelID, InputID: in.ID, ConnectionID: in.ConnectionID, Payload: Payload{Input: &in}})
		}
		err = ir.Err()
		ir.Close()
		if err != nil {
			return nil, err
		}
		b.Revision++
		b.Status = "interrupted"
		b.Incomplete = true
		b.Interrupted = &now
		b.Reason = "process_interrupted"
		events = append(events, Event{Type: "broadcast.interrupted", At: now, Actor: Actor{Source: "system"}, Outcome: "unknown", Reason: b.Reason, Owner: b.Owner, BroadcastID: b.ID, ChannelID: b.ChannelID, Payload: Payload{Broadcast: &b}})
	}
	return events, nil
}
func (m *MySQL) Prune(ctx context.Context, cutoff time.Time) error {
	// Each statement is bounded. Repeat on later ticks rather than locking a
	// whole retention window; events are independent of aggregate lifetimes.
	_, err := m.DB.ExecContext(ctx, `DELETE FROM audit_events WHERE occurred_at < ? LIMIT 1000`, cutoff)
	if err != nil {
		return err
	}
	rows, err := m.DB.QueryContext(ctx, `SELECT broadcast_id FROM broadcasts WHERE (ended_at < ? OR interruption_detected_at < ?) AND status IN ('ended','interrupted') LIMIT 100`, cutoff, cutoff)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		tx, err := m.DB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "DELETE FROM broadcast_inputs WHERE broadcast_id=?", id)
		if err == nil {
			_, err = tx.ExecContext(ctx, "DELETE FROM broadcasts WHERE broadcast_id=?", id)
		}
		if err != nil {
			tx.Rollback()
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// Migrate is an explicit operator command, never called during application startup.
func (m *MySQL) Migrate(ctx context.Context) error {
	conn, err := m.DB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	var lock int
	if err = conn.QueryRowContext(ctx, "SELECT GET_LOCK(CONCAT(DATABASE(),'.mi_audit_migrate'),10)").Scan(&lock); err != nil {
		return err
	}
	if lock != 1 {
		return errors.New("migration already running")
	}
	defer conn.ExecContext(context.Background(), "SELECT RELEASE_LOCK(CONCAT(DATABASE(),'.mi_audit_migrate'))")
	// A completed migration is immutable; inspect it before any DDL.
	var previous string
	checkErr := conn.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE version=1").Scan(&previous)
	if checkErr == nil {
		if previous != Checksum() {
			return errors.New("migration checksum mismatch")
		}
		return nil
	}
	var dbErr *mysql.MySQLError
	if !errors.Is(checkErr, sql.ErrNoRows) && !(errors.As(checkErr, &dbErr) && dbErr.Number == 1146) {
		return checkErr
	}
	var lines []string
	for _, line := range strings.Split(Schema, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") {
			lines = append(lines, line)
		}
	}
	for _, part := range strings.Split(strings.Join(lines, "\n"), ";") {
		if strings.TrimSpace(part) == "" {
			continue
		}
		if _, err = conn.ExecContext(ctx, part); err != nil {
			return err
		}
	}
	var existing string
	err = conn.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE version=1").Scan(&existing)
	if err == nil {
		if existing != Checksum() {
			return errors.New("migration checksum mismatch")
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = conn.ExecContext(ctx, "INSERT INTO schema_migrations(version,checksum) VALUES (1,?)", Checksum())
	return err
}

// Data errors are quarantined. Connectivity, missing schema, deadlocks and
// ambiguous commits remain retryable; never log the SQL error's payload.
func invalidRow(err error) bool {
	var e *mysql.MySQLError
	if !errors.As(err, &e) {
		return false
	}
	switch e.Number {
	case 1048, 1264, 1265, 1292, 1366, 1406, 4025:
		return true
	}
	return false
}
