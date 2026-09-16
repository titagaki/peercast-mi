package site

import (
	"net/http"

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
