package site

import (
	"net/http"
	"strings"

	"github.com/titagaki/peercast-mi/internal/catalog"
	"github.com/titagaki/peercast-pcp/pcp"
)

func (s *Server) channelList(r *http.Request) ([]channelView, []catalog.Status, error) {
	rows := make([]channelView, 0)
	statuses := make([]catalog.Status, 0)
	seen := map[string]bool{}
	if s.catalog != nil {
		entries, status, err := s.catalog.Update(r.Context())
		if err != nil {
			return nil, nil, err
		}
		statuses = status
		for _, entry := range entries {
			id := strings.ToLower(entry.ChannelID)
			if id == strings.Repeat("0", 32) || seen[id] {
				continue
			}
			seen[id] = true
			playable := catalog.PublicTracker(entry.Tracker)
			listeners := -1
			if entry.Listeners != nil {
				listeners = *entry.Listeners
			}
			rows = append(rows, channelView{ID: id, Name: entry.Name, Genre: entry.Genre, Description: entry.Description, Comment: entry.Comment, Uptime: entry.Uptime, ContactURL: entry.ContactURL, ContentType: entry.ContentType, Listeners: listeners, YellowPage: entry.YellowPage, Playable: &playable})
		}
	}
	for _, ch := range s.mgr.List() {
		local := view(ch)
		if !seen[local.ID] {
			continue
		}
		for i := range rows {
			if rows[i].ID != local.ID {
				continue
			}
			// Keep directory metadata while a newly started relay awaits CHAN_INFO.
			if local.Name != "" {
				local.YellowPage = rows[i].YellowPage
				// A local relay only knows its own subtree, not the whole audience.
				// Preserve the YP count, including hidden/unknown values.
				local.Listeners = rows[i].Listeners
				if !ch.IsBroadcasting() {
					local.Uptime = rows[i].Uptime
				}
				rows[i] = local
			}
			playable := true
			rows[i].Playable = &playable
			rows[i].Receiving = local.Receiving
			break
		}
	}
	return rows, statuses, nil
}
func (s *Server) directory(w http.ResponseWriter, r *http.Request, ss *session) {
	rows, statuses, err := s.channelList(r)
	if err != nil {
		http.Error(w, "一覧の取得を中断しました。", 503)
		return
	}
	reply(w, map[string]any{"channels": rows, "sources": statuses})
}

// New relays require a catalog entry, a public tracker, and a reserved viewer
// slot (the caller reserves it first). Never accept client-supplied addresses.
func (s *Server) ensureChannel(w http.ResponseWriter, r *http.Request, id pcp.GnuID) bool {
	s.mutate.Lock()
	defer s.mutate.Unlock()
	if _, ok := s.mgr.GetByID(id); ok {
		return true
	}
	if s.catalog == nil {
		http.Error(w, "配信は終了しました。", 404)
		return false
	}
	rows, _ := s.catalog.Snapshot()
	var tracker string
	for _, row := range rows {
		parsed, err := parseID(row.ChannelID)
		if err == nil && parsed == id && id != (pcp.GnuID{}) {
			tracker = row.Tracker
			break
		}
	}
	if !catalog.PublicTracker(tracker) {
		http.Error(w, "一覧にないか、このサイトから接続できない配信です。一覧を更新してください。", 404)
		return false
	}
	n := 0
	for _, ch := range s.mgr.List() {
		if !ch.IsBroadcasting() {
			n++
		}
	}
	if n >= s.cfg.MaxRelayChannels {
		http.Error(w, "中継チャンネル数の上限です。しばらく待って再試行してください。", 429)
		return false
	}
	if r.Context().Err() != nil {
		return false
	}
	if _, err := s.mgr.StartRelay(id, tracker); err != nil {
		http.Error(w, "中継を開始できません。ノードの上限と設定を確認してください。", 503)
		return false
	}
	return true
}
