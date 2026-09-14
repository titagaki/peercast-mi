package site

import (
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
	s.adminProxy.ServeHTTP(w, r)
}
