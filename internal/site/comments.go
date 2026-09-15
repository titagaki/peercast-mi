package site

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/titagaki/peercast-mi/internal/catalog"
	"golang.org/x/text/encoding/japanese"
)

var digits = regexp.MustCompile(`^[0-9]{1,20}$`)
var shitarabaThread = regexp.MustCompile(`^/bbs/read\.cgi/([a-z]+)/([0-9]+)/([0-9]+)(?:/[^?]*)?$`)
var shitarabaBoard = regexp.MustCompile(`^/([a-z]+)/([0-9]+)/?$`)
var jpnknThread = regexp.MustCompile(`^/test/read\.cgi/([A-Za-z0-9_]+)/([0-9]+)(?:/[^?]*)?$`)
var jpnknBoard = regexp.MustCompile(`^/([A-Za-z0-9_]+)/?$`)
var subjectTitle = regexp.MustCompile(`^(.*)\s*\(([0-9]+)\)\s*$`)
var breakTag = regexp.MustCompile(`(?i)<br\s*/?>`)
var markupTag = regexp.MustCompile(`<[^>]*>`)

type boardAddress struct {
	host, board, thread string
	shitaraba           bool
}

func parseBoard(raw string) (boardAddress, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Port() != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return boardAddress{}, false
	}
	host := strings.ToLower(u.Hostname())
	b := boardAddress{host: host}
	switch host {
	case "jbbs.shitaraba.net", "jbbs.livedoor.jp":
		b.host, b.shitaraba = "jbbs.shitaraba.net", true
		if m := shitarabaThread.FindStringSubmatch(u.Path); m != nil {
			b.board = m[1] + "/" + m[2]
			b.thread = m[3]
		} else if m := shitarabaBoard.FindStringSubmatch(u.Path); m != nil {
			b.board = m[1] + "/" + m[2]
		}
	case "bbs.jpnkn.com":
		if m := jpnknThread.FindStringSubmatch(u.Path); m != nil {
			b.board = m[1]
			b.thread = m[2]
		} else if m := jpnknBoard.FindStringSubmatch(u.Path); m != nil {
			b.board = m[1]
		}
	default:
		return boardAddress{}, false
	}
	return b, b.board != "" && (b.thread == "" || digits.MatchString(b.thread))
}
func (b boardAddress) subjectURL() string {
	return "https://" + b.host + "/" + b.board + "/subject.txt"
}
func (b boardAddress) datURL(thread string) string {
	if b.shitaraba {
		return "https://" + b.host + "/bbs/rawmode.cgi/" + b.board + "/" + thread + "/"
	}
	return "https://" + b.host + "/" + b.board + "/dat/" + thread + ".dat"
}

type bbsThread struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Comments int    `json:"comments"`
}
type bbsComment struct {
	No   int    `json:"no"`
	Name string `json:"name"`
	Date string `json:"date"`
	Body string `json:"body"`
}
type boardView struct {
	Supported    bool         `json:"supported"`
	ThreadID     string       `json:"threadId"`
	ThreadTitle  string       `json:"threadTitle"`
	CommentCount int          `json:"commentCount"`
	Threads      []bbsThread  `json:"threads"`
	Comments     []bbsComment `json:"comments"`
}
type boardCacheEntry struct {
	done    chan struct{}
	expires time.Time
	text    string
	err     error
}
type boardReader struct {
	mu     sync.Mutex
	cache  map[string]*boardCacheEntry
	slots  chan struct{}
	client *http.Client
}

func newBoardReader() *boardReader {
	return &boardReader{cache: make(map[string]*boardCacheEntry), slots: make(chan struct{}, 4), client: &http.Client{
		Timeout:       5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport:     &http.Transport{Proxy: nil, DialContext: publicBoardDial, ResponseHeaderTimeout: 4 * time.Second},
	}}
}
func publicBoardDial(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if host != "jbbs.shitaraba.net" && host != "bbs.jpnkn.com" {
		return nil, errors.New("unsupported board host")
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	for _, ip := range ips {
		if !catalog.PublicTracker(net.JoinHostPort(ip.IP.String(), port)) {
			return nil, errors.New("non-public board address")
		}
	}
	for _, ip := range ips {
		conn, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
		if err == nil {
			return conn, nil
		}
	}
	return nil, errors.New("board connection failed")
}

// Cache errors as well as data; one request per URL per 10 seconds, at most
// 64 cache entries and 4 in-flight network reads. No viewer cookies are sent.
func (b *boardReader) read(ctx context.Context, target string, shitaraba bool) (string, error) {
	b.mu.Lock()
	now := time.Now()
	for key, e := range b.cache {
		if !e.expires.IsZero() && !now.Before(e.expires) {
			delete(b.cache, key)
		}
	}
	e := b.cache[target]
	if e == nil {
		if len(b.cache) >= 64 {
			b.mu.Unlock()
			return "", errors.New("board cache full")
		}
		e = &boardCacheEntry{done: make(chan struct{})}
		b.cache[target] = e
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var data string
			var err error
			select {
			case b.slots <- struct{}{}:
				data, err = b.fetch(ctx, target, shitaraba)
				<-b.slots
			case <-ctx.Done():
				err = ctx.Err()
			}
			b.mu.Lock()
			e.text, e.err, e.expires = data, err, time.Now().Add(10*time.Second)
			close(e.done)
			b.mu.Unlock()
		}()
	}
	b.mu.Unlock()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-e.done:
		return e.text, e.err
	}
}
func (b *boardReader) fetch(ctx context.Context, target string, shitaraba bool) (string, error) {
	r, err := http.NewRequestWithContext(ctx, "GET", target, nil)
	if err != nil {
		return "", err
	}
	r.Header.Set("Accept", "text/plain")
	resp, err := b.client.Do(r)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/html") {
		return "", errors.New("board response unavailable")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
	if err != nil {
		return "", err
	}
	if len(data) > 2<<20 {
		return "", errors.New("board response too large")
	}
	charset := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(charset, "euc-jp") || (!utf8.Valid(data) && shitaraba && !strings.Contains(charset, "shift")) {
		data, err = japanese.EUCJP.NewDecoder().Bytes(data)
	} else if strings.Contains(charset, "shift") || !utf8.Valid(data) {
		data, err = japanese.ShiftJIS.NewDecoder().Bytes(data)
	}
	if err != nil {
		return "", err
	}
	s := strings.TrimPrefix(string(data), "\ufeff")
	start := strings.ToLower(strings.TrimSpace(s))
	if strings.HasPrefix(start, "<!doctype") || strings.HasPrefix(start, "<html") {
		return "", errors.New("HTML instead of board data")
	}
	return s, nil
}
func plainComment(s string) string {
	return strings.TrimSpace(html.UnescapeString(markupTag.ReplaceAllString(breakTag.ReplaceAllString(s, "\n"), "")))
}
func parseSubjects(body string, shitaraba bool) ([]bbsThread, error) {
	return parseSubjectsLimit(body, shitaraba, 200)
}
func parseSubjectsLimit(body string, shitaraba bool, limit int) ([]bbsThread, error) {
	rows := make([]bbsThread, 0)
	seen := map[string]bool{}
	scan := bufio.NewScanner(strings.NewReader(body))
	scan.Buffer(make([]byte, 4096), 256<<10)
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if line == "" {
			continue
		}
		sep, ext := "<>", ".dat"
		if shitaraba {
			sep, ext = ",", ".cgi"
		}
		p := strings.SplitN(line, sep, 2)
		if len(p) != 2 {
			continue
		}
		id := strings.TrimSuffix(p[0], ext)
		if id == p[0] || !digits.MatchString(id) || seen[id] {
			continue
		}
		m := subjectTitle.FindStringSubmatch(p[1])
		if m == nil {
			continue
		}
		count, _ := strconv.Atoi(m[2])
		seen[id] = true
		rows = append(rows, bbsThread{id, plainComment(m[1]), count})
		if limit > 0 && len(rows) == limit {
			break
		}
	}
	return rows, scan.Err()
}
func parseComments(body string, shitaraba bool) ([]bbsComment, string, int, error) {
	rows := make([]bbsComment, 0, 30)
	title := ""
	count := 0
	scan := bufio.NewScanner(strings.NewReader(body))
	scan.Buffer(make([]byte, 4096), 256<<10)
	for scan.Scan() {
		p := strings.Split(scan.Text(), "<>")
		if len(p) < 5 {
			return nil, "", 0, errors.New("invalid comment data")
		}
		var c bbsComment
		if shitaraba {
			if len(p) < 6 {
				return nil, "", 0, errors.New("invalid rawmode data")
			}
			n, err := strconv.Atoi(p[0])
			if err != nil || n < 1 {
				return nil, "", 0, errors.New("invalid comment number")
			}
			c = bbsComment{n, plainComment(p[1]), plainComment(p[3]), plainComment(p[4])}
			if title == "" {
				title = plainComment(p[5])
			}
		} else {
			c = bbsComment{count + 1, plainComment(p[0]), plainComment(p[2]), plainComment(p[3])}
			if title == "" {
				title = plainComment(p[4])
			}
		}
		count = c.No
		rows = append(rows, c)
		if len(rows) > 30 {
			copy(rows, rows[1:])
			rows = rows[:30]
		}
	}
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	return rows, title, count, scan.Err()
}

func (s *Server) comments(w http.ResponseWriter, r *http.Request, ss *session) {
	id, err := parseID(r.PathValue("id"))
	q, queryErr := url.ParseQuery(r.URL.RawQuery)
	thread := q.Get("thread")
	if err != nil || queryErr != nil || len(q) > 1 || (len(q) == 1 && len(q["thread"]) != 1) || (thread != "" && !digits.MatchString(thread)) {
		http.Error(w, "掲示板の指定が不正です。", 400)
		return
	}
	// Resolve the contact URL from trusted node/catalog state, never a URL supplied by a viewer.
	contact := ""
	found := false
	if ch, ok := s.mgr.GetByID(id); ok {
		contact = ch.Info().URL
		found = true
	}
	if contact == "" && s.catalog != nil {
		rows, _, err := s.catalog.Update(r.Context())
		if err != nil {
			http.Error(w, "チャンネルを確認できません。", 503)
			return
		}
		for _, row := range rows {
			parsed, e := parseID(row.ChannelID)
			if e == nil && parsed == id && row.ChannelID != strings.Repeat("0", 32) {
				contact = row.ContactURL
				found = true
				break
			}
		}
	}
	if !found {
		http.Error(w, "チャンネルが見つかりません。", 404)
		return
	}
	board, supported := parseBoard(contact)
	view := boardView{Supported: supported, Threads: []bbsThread{}, Comments: []bbsComment{}}
	if !supported {
		reply(w, view)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 11*time.Second)
	defer cancel()
	subject, err := s.boards.read(ctx, board.subjectURL(), board.shitaraba)
	if err == nil {
		view.Threads, err = parseSubjects(subject, board.shitaraba)
	}
	if err != nil {
		http.Error(w, "掲示板を取得できません。しばらく待って再試行してください。", 502)
		return
	}
	if thread == "" {
		thread = board.thread
	}
	// A selector may only address a thread from this board's subject list, or
	// the exact thread already named by the broadcaster's contact URL.
	allowed := thread == "" || thread == board.thread
	for _, t := range view.Threads {
		if t.ID == thread {
			allowed = true
		}
	}
	if !allowed {
		http.Error(w, "スレッドが見つかりません。", 404)
		return
	}
	view.ThreadID = thread
	if thread != "" {
		body, e := s.boards.read(ctx, board.datURL(thread), board.shitaraba)
		if e == nil {
			view.Comments, view.ThreadTitle, view.CommentCount, e = parseComments(body, board.shitaraba)
		}
		if e != nil {
			http.Error(w, "コメントを取得できません。しばらく待って再試行してください。", 502)
			return
		}
	}
	reply(w, view)
}

// safeReturnPath prevents login return URLs from becoming an open redirect.
func safeReturnPath(path string) string {
	if path == "/broadcast" || path == "/" {
		return path
	}
	if strings.HasPrefix(path, "/channels/") {
		if id, err := parseID(strings.TrimPrefix(path, "/channels/")); err == nil {
			return fmt.Sprintf("/channels/%x", id[:])
		}
	}
	return "/"
}
