package site

import (
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/titagaki/peercast-mi/internal/channel"
)

// Never serialize Channel or administrative RPC status: they can contain
// stream keys, source URLs and network addresses belonging to other users.
type channelView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Genre       string `json:"genre"`
	Description string `json:"description"`
	Comment     string `json:"comment,omitempty"`
	Uptime      *int   `json:"uptime,omitempty"`
	ContactURL  string `json:"contactUrl"`
	ContentType string `json:"contentType"`
	Bitrate     uint32 `json:"bitrate"`
	Receiving   bool   `json:"receiving"`
	Listeners   int    `json:"listeners"`
	YellowPage  string `json:"yellowPage,omitempty"`
	Playable    *bool  `json:"playable,omitempty"`
}

func view(ch *channel.Channel) channelView {
	i := ch.Info()
	v := channelView{ID: hex.EncodeToString(ch.ID[:]), Name: i.Name, Genre: i.Genre, Description: i.Desc, Comment: i.Comment, ContactURL: i.URL, ContentType: i.Type, Bitrate: i.Bitrate, Receiving: ch.IsReceiving(), Listeners: ch.TotalListeners()}
	// A local relay's age is not the broadcaster's uptime.
	if ch.IsBroadcasting() {
		uptime := int(ch.UptimeSeconds())
		v.Uptime = &uptime
	}
	return v
}
func (s *Server) channels(w http.ResponseWriter, r *http.Request, ss *session) {
	rows, _, err := s.channelList(r)
	if err != nil {
		http.Error(w, "一覧の取得を中断しました。", 503)
		return
	}
	reply(w, rows)
}
func account(ss *session) string {
	if ss.User.ID == "dev-local" {
		return "site:dev:local"
	}
	return "site:x:" + ss.User.ID
}
func (s *Server) key(ss *session) string {
	for _, e := range s.mgr.ListStreamKeys() {
		if e.AccountName == account(ss) {
			return e.StreamKey
		}
	}
	return ""
}
func (s *Server) broadcastInfo(w http.ResponseWriter, r *http.Request, ss *session) {
	s.mutate.Lock()
	defer s.mutate.Unlock()
	key := s.key(ss)
	var own *channelView
	if ch, ok := s.mgr.GetByStreamKey(key); ok {
		v := view(ch)
		own = &v
	}
	reply(w, map[string]any{"streamKey": key, "rtmpUrl": s.cfg.RTMPURL, "channel": own})
}
func (s *Server) rotateKey(w http.ResponseWriter, r *http.Request, ss *session) {
	s.mutate.Lock()
	defer s.mutate.Unlock()
	old := s.key(ss)
	if _, active := s.mgr.GetByStreamKey(old); active {
		http.Error(w, "配信を停止してからキーを再発行してください。", 409)
		return
	}
	if old == "" && len(s.mgr.ListStreamKeys()) >= 10000 {
		http.Error(w, "キー発行数の上限です。管理者に連絡してください。", 503)
		return
	}
	key := randomToken()
	if err := s.mgr.IssueStreamKey(account(ss), key); err != nil {
		http.Error(w, "キーを保存できませんでした。", 500)
		return
	}
	reply(w, map[string]string{"streamKey": key})
}
func (s *Server) broadcast(w http.ResponseWriter, r *http.Request, ss *session) {
	var input struct {
		Name        string `json:"name"`
		Genre       string `json:"genre"`
		Description string `json:"description"`
		Comment     string `json:"comment"`
		ContactURL  string `json:"contactUrl"`
		Bitrate     int64  `json:"bitrate"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(&input); err != nil {
		http.Error(w, "配信設定が不正です。", 400)
		return
	}
	if err := d.Decode(new(any)); err != io.EOF {
		http.Error(w, "配信設定が不正です。", 400)
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 256 || len(input.Genre) > 256 || len(input.Description) > 2048 || len(input.Comment) > 2048 || len(input.ContactURL) > 2048 || input.Bitrate < 0 || input.Bitrate > 2147483647 {
		http.Error(w, "名前は必須です。入力の長さも確認してください。", 400)
		return
	}
	input.ContactURL = strings.TrimSpace(input.ContactURL)
	if input.ContactURL != "" {
		u, err := url.Parse(input.ContactURL)
		if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
			http.Error(w, "URLはHTTPまたはHTTPSのURLを入力してください。", http.StatusBadRequest)
			return
		}
	}
	s.mutate.Lock()
	defer s.mutate.Unlock()
	key := s.key(ss)
	if key == "" {
		http.Error(w, "先に配信キーを発行してください。", 409)
		return
	}
	if _, active := s.mgr.GetByStreamKey(key); active {
		http.Error(w, "既に配信枠があります。停止後に作成してください。", 409)
		return
	}
	ch, err := s.mgr.Broadcast(key, channel.ChannelInfo{Name: input.Name, Genre: input.Genre, Desc: input.Description, Comment: input.Comment, URL: input.ContactURL, Bitrate: uint32(input.Bitrate), Type: "FLV", MIMEType: "video/x-flv", Ext: ".flv"}, channel.TrackInfo{})
	if err != nil {
		http.Error(w, "配信枠を作成できませんでした。", 409)
		return
	}
	if s.bump != nil {
		s.bump()
	}
	reply(w, view(ch))
}
func (s *Server) stopBroadcast(w http.ResponseWriter, r *http.Request, ss *session) {
	s.mutate.Lock()
	defer s.mutate.Unlock()
	// No caller-supplied account name, stream key or channel ID is accepted.
	if ch, ok := s.mgr.GetByStreamKey(s.key(ss)); ok {
		s.mgr.Stop(ch.ID)
	}
	if s.bump != nil {
		s.bump()
	}
	reply(w, map[string]bool{"ok": true})
}
