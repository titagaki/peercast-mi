package jsonrpc

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/titagaki/peercast-mi/internal/config"
	"github.com/titagaki/peercast-pcp/pcp"
)

func TestUpdateYPChannels(t *testing.T) {
	yp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("配信<>abcdef0123456789abcdef0123456789<>8.8.8.8:7144<><>ゲーム<>説明<>-1<>-1<>1200<>FLV<>作者<>album<>track<><>encoded<>1:30<>click<>コメント<>0\n"))
	}))
	defer yp.Close()
	s := New(pcp.GnuID{}, nil, &config.Config{YPs: []config.YP{{Name: "YP", ChannelsURL: yp.URL}}}, nil)
	resp := assertResult(t, rpcCall(t, s, "updateYPChannels", nil)).([]interface{})
	if len(resp) != 1 {
		t.Fatal(resp)
	}
	row := resp[0].(map[string]interface{})
	if row["channelId"] != "ABCDEF0123456789ABCDEF0123456789" || row["yellowPage"] != "YP" || row["creator"] != "作者" || row["uptime"] != float64(5400) {
		t.Fatal(row)
	}
	s = New(pcp.GnuID{}, nil, &config.Config{}, nil)
	if len(assertResult(t, rpcCall(t, s, "updateYPChannels", nil)).([]interface{})) != 0 {
		t.Fatal("empty sources must return []")
	}
}
