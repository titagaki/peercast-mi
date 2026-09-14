package servent

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/titagaki/peercast-mi/internal/channel"
	"github.com/titagaki/peercast-pcp/pcp"
)

func TestSiteGateHTTPOnly(t *testing.T) {
	for _, test := range []struct {
		path, auth string
		want       int
	}{
		{"/stream/00000000000000000000000000000001", "", 403},
		{"/pls/00000000000000000000000000000001", "", 403},
		{"/stream/00000000000000000000000000000001", "Bearer wrong", 403},
		{"/stream/00000000000000000000000000000001", "Bearer internal", 404},
		{"/channel/00000000000000000000000000000001", "", 404},
	} {
		t.Run(test.path+test.auth, func(t *testing.T) {
			mgr := channel.NewManager(pcp.GnuID{})
			defer mgr.StopAll()
			l := NewListener(pcp.GnuID{}, mgr, 7144, 0, 0, 0, 0)
			l.ViewerToken = "internal"
			server, client := net.Pipe()
			defer client.Close()
			client.SetDeadline(time.Now().Add(time.Second))
			done := make(chan struct{})
			go func() { l.handle(server); close(done) }()
			_, err := io.WriteString(client, "GET "+test.path+" HTTP/1.0\r\nHost: localhost\r\nAuthorization: "+test.auth+"\r\n\r\n")
			if err != nil {
				t.Fatal(err)
			}
			resp, err := http.ReadResponse(bufio.NewReader(client), nil)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != test.want {
				t.Fatal(resp.StatusCode, test.want)
			}
			client.Close()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("handler did not close")
			}
		})
	}
}
