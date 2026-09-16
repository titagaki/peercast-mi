package site

import (
	"github.com/titagaki/peercast-mi/internal/audit"
	"mime"
	"net/http"
)

// Development sessions never acquire administrative privileges via this route.
func (s *Server) isAdmin(ss *session) bool {
	return ss != nil && !s.cfg.DevLogin && s.adminIDs[ss.User.ID]
}

func (s *Server) adminRPC(w http.ResponseWriter, r *http.Request, ss *session) {
	if !s.isAdmin(ss) {
		http.Error(w, "このアカウントには管理権限がありません。", http.StatusForbidden)
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		http.Error(w, "JSON request required", http.StatusUnsupportedMediaType)
		return
	}
	if r.URL.RawQuery != "" {
		http.Error(w, "invalid query", http.StatusBadRequest)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if s.adminHandler != nil {
		forwarded := r.Clone(audit.WithActor(r.Context(), s.auditActor(r, ss)))
		forwarded.RemoteAddr = "127.0.0.1:0"
		forwarded.URL.Path = "/api/1"
		forwarded.URL.RawPath = ""
		forwarded.Header = make(http.Header)
		forwarded.Header.Set("Content-Type", "application/json")
		s.adminHandler.ServeHTTP(w, forwarded)
		return
	}
	s.adminProxy.ServeHTTP(w, r)
}

// SetAdminHandler is configured before serving requests.
func (s *Server) SetAdminHandler(h http.Handler) { s.adminHandler = h }
