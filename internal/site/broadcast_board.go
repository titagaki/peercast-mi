package site

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func (b boardAddress) boardURL() string { return "https://" + b.host + "/" + b.board + "/" }
func (b boardAddress) threadURL(id string) string {
	if b.shitaraba {
		return "https://" + b.host + "/bbs/read.cgi/" + b.board + "/" + id + "/"
	}
	return "https://" + b.host + "/test/read.cgi/" + b.board + "/" + id + "/"
}
func (b boardAddress) settingsURL() string {
	if b.shitaraba {
		return "https://" + b.host + "/bbs/api/setting.cgi/" + b.board + "/"
	}
	return b.boardURL() + "SETTING.TXT"
}

type broadcastBoardView struct {
	Supported       bool       `json:"supported"`
	BoardTitle      string     `json:"boardTitle"`
	BoardURL        string     `json:"boardUrl"`
	Thread          *bbsThread `json:"thread"`
	ThreadURL       string     `json:"threadUrl"`
	LatestThread    *bbsThread `json:"latestThread"`
	LatestThreadURL string     `json:"latestThreadUrl"`
	ThreadError     string     `json:"threadError,omitempty"`
}

func newerThread(a, b string) bool {
	a, b = strings.TrimLeft(a, "0"), strings.TrimLeft(b, "0")
	return len(a) > len(b) || (len(a) == len(b) && a > b)
}

// Form URLs use the same strict host/path allowlist and public-address dialer
// as comment viewing. Never fetch an arbitrary contact URL or follow redirects.
func (s *Server) broadcastBoard(w http.ResponseWriter, r *http.Request, ss *session) {
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(q) != 1 || len(q["url"]) != 1 || len(q.Get("url")) > 2048 {
		http.Error(w, "掲示板の指定が不正です。", 400)
		return
	}
	board, ok := parseBoard(q.Get("url"))
	v := broadcastBoardView{Supported: ok}
	if !ok {
		reply(w, v)
		return
	}
	v.BoardURL = board.boardURL()
	ctx, cancel := context.WithTimeout(r.Context(), 11*time.Second)
	defer cancel()
	settings, err := s.boards.read(ctx, board.settingsURL(), board.shitaraba)
	if err != nil {
		http.Error(w, "掲示板の設定を取得できません。", 502)
		return
	}
	limit := 1000
	for _, line := range strings.Split(settings, "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if !found {
			continue
		}
		switch key {
		case "BBS_TITLE":
			v.BoardTitle = plainComment(value)
		case "BBS_THREAD_STOP":
			if n, e := strconv.Atoi(value); e == nil && n > 0 {
				limit = n
			}
		case "ERROR":
			http.Error(w, "掲示板を利用できません。", 502)
			return
		}
	}
	if v.BoardTitle == "" {
		http.Error(w, "掲示板の設定形式が不正です。", 502)
		return
	}
	subject, err := s.boards.read(ctx, board.subjectURL(), board.shitaraba)
	if err != nil {
		http.Error(w, "スレッド一覧を取得できません。", 502)
		return
	}
	// Read the whole bounded subject response: the newest thread need not be
	// among the first 200 bumped threads shown in the viewing-page selector.
	threads, err := parseSubjectsLimit(subject, board.shitaraba, 0)
	if err != nil {
		http.Error(w, "スレッド一覧の形式が不正です。", 502)
		return
	}
	for _, t := range threads {
		if t.ID == board.thread {
			current := t
			v.Thread = &current
			v.ThreadURL = board.threadURL(t.ID)
		}
		if t.Comments < limit && (v.LatestThread == nil || newerThread(t.ID, v.LatestThread.ID)) {
			latest := t
			v.LatestThread = &latest
			v.LatestThreadURL = board.threadURL(t.ID)
		}
	}
	// Full/archived threads can disappear from subject.txt. Still show their
	// title/count when readable, and keep the new-thread action available.
	if board.thread != "" && v.Thread == nil {
		body, e := s.boards.read(ctx, board.datURL(board.thread), board.shitaraba)
		if e == nil {
			_, title, count, parseErr := parseComments(body, board.shitaraba)
			e = parseErr
			if e == nil {
				v.Thread = &bbsThread{board.thread, title, count}
				v.ThreadURL = board.threadURL(board.thread)
			}
		}
		if e != nil {
			v.ThreadError = "指定したスレッドの情報を取得できません。"
		}
	}
	reply(w, v)
}
