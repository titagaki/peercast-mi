package site

import (
	"context"
	"net/http"
	"time"

	"github.com/titagaki/peercast-mi/internal/audit"
)

type auditResponse struct {
	http.ResponseWriter
	code int
}

func (w *auditResponse) WriteHeader(code int) { w.code = code; w.ResponseWriter.WriteHeader(code) }
func (s *Server) auditActor(r *http.Request, ss *session) audit.Actor {
	a := audit.Actor{Source: "site", IP: s.broadcastClientIP(r)}
	if s.cfg.DevLogin {
		a.Source = "development"
	}
	if ss != nil {
		a.Account = account(ss)
		a.Name = ss.User.Name
		a.Session = ss.AuditRef
	}
	return a
}
func (s *Server) auditStatus(w http.ResponseWriter, r *http.Request, ss *session) {
	if !s.isAdmin(ss) {
		http.Error(w, "このアカウントには管理権限がありません。", 403)
		return
	}
	if recorder, ok := s.mgr.Audit.(*audit.Recorder); ok {
		reply(w, recorder.Status())
	} else {
		reply(w, audit.Status{})
	}
}

// SetAuditReader is configured before serving. Keep read traffic off the writer pool.
func (s *Server) SetAuditReader(reader audit.Reader) {
	s.auditReader = reader
	s.auditReadSlots = make(chan struct{}, 2)
}
func (s *Server) auditQuery(w http.ResponseWriter, r *http.Request, ss *session, kind string, query func(context.Context, audit.Filter) (any, error)) {
	if !s.isAdmin(ss) {
		http.Error(w, "このアカウントには管理権限がありません。", http.StatusForbidden)
		return
	}
	f, err := audit.ParseFilter(kind, r.URL.Query())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if s.auditReader == nil {
		http.Error(w, "ログ閲覧用のDB接続が設定されていません。", http.StatusServiceUnavailable)
		return
	}
	select {
	case s.auditReadSlots <- struct{}{}:
		defer func() { <-s.auditReadSlots }()
	default:
		http.Error(w, "ログの検索が混み合っています。少し待って再試行してください。", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	data, err := query(ctx, f)
	if err != nil {
		http.Error(w, "ログを取得できませんでした。DB接続とテーブルの状態を確認してください。", http.StatusServiceUnavailable)
		return
	}
	reply(w, data)
}
func (s *Server) auditEvents(w http.ResponseWriter, r *http.Request, ss *session) {
	s.auditQuery(w, r, ss, "events", func(ctx context.Context, f audit.Filter) (any, error) { return s.auditReader.Events(ctx, f) })
}
func (s *Server) auditBroadcasts(w http.ResponseWriter, r *http.Request, ss *session) {
	s.auditQuery(w, r, ss, "broadcasts", func(ctx context.Context, f audit.Filter) (any, error) { return s.auditReader.Broadcasts(ctx, f) })
}
func (s *Server) auditInputs(w http.ResponseWriter, r *http.Request, ss *session) {
	id := r.PathValue("id")
	s.auditQuery(w, r, ss, "inputs:"+id, func(ctx context.Context, f audit.Filter) (any, error) { return s.auditReader.Inputs(ctx, id, f) })
}
