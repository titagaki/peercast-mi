package site

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/titagaki/peercast-mi/internal/catalog"
	"github.com/titagaki/peercast-mi/internal/channel"
	"github.com/titagaki/peercast-mi/internal/config"
)

type directoryRelay struct {
	stop chan struct{}
	once sync.Once
}

func TestLocalPresentationMetadata(t *testing.T) {
	s := testSite(t)
	s.mgr.IssueStreamKey("presentation", "presentation-test-key")
	ch, err := s.mgr.Broadcast("presentation-test-key", channel.ChannelInfo{Name: "Live", Comment: "配信者コメント"}, channel.TrackInfo{})
	if err != nil {
		t.Fatal(err)
	}
	v := view(ch)
	if v.Comment != "配信者コメント" || v.Uptime == nil || *v.Uptime < 0 {
		t.Fatal(v)
	}
	unknownRelay := channel.New(ch.ID, ch.BroadcastID(), 1024)
	if view(unknownRelay).Uptime != nil {
		t.Fatal("local relay age must not be presented as broadcast uptime")
	}
}

func (r *directoryRelay) Run()                { <-r.stop }
func (r *directoryRelay) Stop()               { r.once.Do(func() { close(r.stop) }) }
func (r *directoryRelay) SetGlobalIP(uint32)  {}
func (r *directoryRelay) SetOnStopped(func()) {}

func TestDirectoryToAuthenticatedRelay(t *testing.T) {
	s := testSite(t)
	addSession(s, "alice", "1")
	const channelID = "abcdef0123456789abcdef0123456789"
	var fetches, starts atomic.Int32
	yp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches.Add(1)
		io.WriteString(w, "配信<>"+channelID+"<>8.8.8.8:7144<>https://example.test<>ゲーム<>説明<>-1<>-1<>1500<>FLV<><><><><>encoded<>1:00<>click<>コメント<>1\n")
		io.WriteString(w, "お知らせ<>"+strings.Repeat("0", 32)+"<><><><><>-9<>-9<>0<>RAW\n")
		io.WriteString(w, "private<>"+strings.Repeat("1", 32)+"<>127.0.0.1:7144<><><>説明<>1<>1<>1500<>FLV\n")
	}))
	defer yp.Close()
	s.SetCatalog(catalog.New([]config.YP{{Name: "YP", ChannelsURL: yp.URL}, {Name: "disabled"}}))
	s.mgr.NewRelay = func(ch *channel.Channel, addr string) channel.RelayHandle {
		starts.Add(1)
		if addr != "8.8.8.8:7144" {
			t.Error("untrusted tracker", addr)
		}
		return &directoryRelay{stop: make(chan struct{})}
	}
	if w := call(s, "GET", "/site/api/directory", "", "", ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if fetches.Load() != 0 {
		t.Fatal("anonymous triggered fetch")
	}
	w := call(s, "GET", "/site/api/directory", "alice", "", "")
	var directory struct {
		Channels []channelView
		Sources  []catalog.Status
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &directory) != nil || len(directory.Channels) != 2 || len(directory.Sources) != 2 {
		t.Fatal(w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "8.8.8.8") || directory.Channels[0].ID != channelID || *directory.Channels[1].Playable {
		t.Fatal(w.Body.String())
	}
	if len(s.mgr.List()) != 0 || starts.Load() != 0 {
		t.Fatal("listing started relay")
	}
	if directory.Channels[0].Comment != "コメント" || directory.Channels[0].Uptime == nil || *directory.Channels[0].Uptime != 3600 || directory.Channels[1].Uptime != nil {
		t.Fatal("presentation metadata lost", w.Body.String())
	}
	if w := call(s, "GET", "/site/stream/"+channelID+"?tip=127.0.0.1:80", "alice", "", ""); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := call(s, "GET", "/site/stream/"+strings.Repeat("1", 32), "alice", "", ""); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w := call(s, "GET", "/site/stream/"+strings.Repeat("2", 32), "alice", "", ""); w.Code != 404 {
		t.Fatal(w.Code)
	}
	s.mu.Lock()
	s.total = s.cfg.MaxViewers
	s.mu.Unlock()
	if w := call(s, "GET", "/site/stream/"+channelID, "alice", "", ""); w.Code != 429 {
		t.Fatal(w.Code)
	}
	if starts.Load() != 0 {
		t.Fatal("quota bypass")
	}
	s.mu.Lock()
	s.total = 0
	s.mu.Unlock()
	s.proxy.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/stream/"+channelID || r.URL.RawQuery != "" || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "Bearer "+s.ViewerToken() {
			t.Error("incorrect proxy request")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"video/x-flv"}}, Body: io.NopCloser(strings.NewReader("FLV-test"))}, nil
	})
	for i := 0; i < 2; i++ {
		w = call(s, "GET", "/site/stream/"+channelID, "alice", "", "")
		if w.Code != 200 || w.Body.String() != "FLV-test" {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if starts.Load() != 1 {
		t.Fatal("duplicate relay", starts.Load())
	}
	w = call(s, "GET", "/site/api/directory", "alice", "", "")
	if json.Unmarshal(w.Body.Bytes(), &directory) != nil || len(directory.Channels) != 2 || directory.Channels[0].Name != "配信" || !*directory.Channels[0].Playable {
		t.Fatal("metadata lost while connecting", w.Body.String())
	}
	if fetches.Load() != 1 {
		t.Fatal("cache not shared")
	}
	parsedID, err := parseID(channelID)
	if err != nil {
		t.Fatal(err)
	}
	ch, _ := s.mgr.GetByID(parsedID)
	info := ch.Info()
	info.Name = "更新された配信"
	info.Comment = "更新コメント"
	ch.SetInfo(info)
	w = call(s, "GET", "/site/api/directory", "alice", "", "")
	if json.Unmarshal(w.Body.Bytes(), &directory) != nil || directory.Channels[0].Comment != "更新コメント" || directory.Channels[0].Uptime == nil || *directory.Channels[0].Uptime != 3600 {
		t.Fatal("relay replaced broadcast uptime", w.Body.String())
	}
}

func TestSiteRelayChannelCap(t *testing.T) {
	s := testSite(t)
	addSession(s, "alice", "1")
	s.cfg.MaxRelayChannels = 1
	s.mgr.NewRelay = func(*channel.Channel, string) channel.RelayHandle { return &directoryRelay{stop: make(chan struct{})} }
	first, _ := parseID(strings.Repeat("1", 32))
	s.mgr.StartRelay(first, "8.8.8.8:7144")
	yp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "next<>"+strings.Repeat("2", 32)+"<>8.8.4.4:7144<><><><>1<>1<>1000<>FLV\n")
	}))
	defer yp.Close()
	s.SetCatalog(catalog.New([]config.YP{{Name: "YP", ChannelsURL: yp.URL}}))
	call(s, "GET", "/site/api/directory", "alice", "", "")
	if w := call(s, "GET", "/site/stream/"+strings.Repeat("2", 32), "alice", "", ""); w.Code != 429 {
		t.Fatal(w.Code, w.Body.String())
	}
	if len(s.mgr.List()) != 1 {
		t.Fatal("relay cap bypassed")
	}
}
