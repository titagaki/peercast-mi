package relay

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/titagaki/peercast-pcp/pcp"
)

func testHostAtom(sid pcp.GnuID, ip uint32, port uint16, cid pcp.GnuID, flags byte) *pcp.Atom {
	return pcp.NewParentAtom(pcp.PCPHost,
		pcp.NewIDAtom(pcp.PCPHostID, sid),
		pcp.NewIntAtom(pcp.PCPHostIP, ip),
		pcp.NewShortAtom(pcp.PCPHostPort, port),
		pcp.NewIntAtom(pcp.PCPHostIP, 0x0a000001),
		pcp.NewShortAtom(pcp.PCPHostPort, port),
		pcp.NewIDAtom(pcp.PCPHostChanID, cid),
		pcp.NewByteAtom(pcp.PCPHostFlags1, flags),
	)
}

// fakeYP は 1 接続を受け付け、GET + helo を読んでから status を返す。503 なら oleh と
// atoms を送って quit で締める。
func fakeYP(t *testing.T, status string, atoms ...*pcp.Atom) (addr string, done <-chan error) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		defer ln.Close()
		conn, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		br := bufio.NewReader(conn)
		line, err := br.ReadString('\n')
		if err != nil || !strings.HasPrefix(line, "GET /channel/") {
			errCh <- errors.New("bad request line: " + line)
			return
		}
		for line != "\r\n" {
			if line, err = br.ReadString('\n'); err != nil {
				errCh <- err
				return
			}
		}
		helo, err := pcp.ReadAtom(br)
		if err != nil || helo.Tag != pcp.PCPHelo {
			errCh <- errors.New("expected helo")
			return
		}
		io.WriteString(conn, "HTTP/1.0 "+status+"\r\nContent-Type: application/x-peercast-pcp\r\n\r\n")
		if !strings.HasPrefix(status, "503") {
			errCh <- nil
			return
		}
		pcp.NewParentAtom(pcp.PCPOleh, pcp.NewIDAtom(pcp.PCPHeloSessionID, pcp.GnuID{0xEE})).Write(conn)
		for _, a := range atoms {
			a.Write(conn)
		}
		pcp.NewIntAtom(pcp.PCPQuit, pcp.PCPErrorQuit+pcp.PCPErrorUnavailable).Write(conn)
		errCh <- nil
	}()
	return ln.Addr().String(), errCh
}

func TestFindTracker_PicksTrackerFromHostList(t *testing.T) {
	cid := pcp.GnuID{0xC1}
	yp, done := fakeYP(t, "503 Service Unavailable",
		testHostAtom(pcp.GnuID{1}, 0xCB007101, 7144, cid, pcp.PCPHostFlags1Relay),                          // relay, not tracker
		testHostAtom(pcp.GnuID{2}, 0xCB007102, 7144, pcp.GnuID{0xC2}, pcp.PCPHostFlags1Tracker),            // other channel
		testHostAtom(pcp.GnuID{3}, 0xCB007103, 7145, cid, pcp.PCPHostFlags1Tracker|pcp.PCPHostFlags1Relay), // tracker
	)
	addr, err := FindTracker(context.Background(), []string{yp}, cid, pcp.GnuID{9}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if addr != "203.0.113.3:7145" {
		t.Fatalf("tracker = %q, want 203.0.113.3:7145", addr)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestFindTracker_SameNATUsesLocalAddr(t *testing.T) {
	cid := pcp.GnuID{0xC1}
	yp, _ := fakeYP(t, "503 Service Unavailable",
		testHostAtom(pcp.GnuID{3}, 0xCB007103, 7145, cid, pcp.PCPHostFlags1Tracker))
	addr, err := FindTracker(context.Background(), []string{yp}, cid, pcp.GnuID{9}, 0xCB007103)
	if err != nil {
		t.Fatal(err)
	}
	if addr != "10.0.0.1:7145" {
		t.Fatalf("tracker = %q, want local address 10.0.0.1:7145", addr)
	}
}

func TestFindTracker_200MeansYPIsTracker(t *testing.T) {
	yp, _ := fakeYP(t, "200 OK")
	addr, err := FindTracker(context.Background(), []string{yp}, pcp.GnuID{0xC1}, pcp.GnuID{9}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if addr != yp {
		t.Fatalf("tracker = %q, want the YP itself %q", addr, yp)
	}
}

func TestFindTracker_FallsThroughYPs(t *testing.T) {
	cid := pcp.GnuID{0xC1}
	notFound, _ := fakeYP(t, "404 Not Found")
	noTracker, _ := fakeYP(t, "503 Service Unavailable",
		testHostAtom(pcp.GnuID{1}, 0xCB007101, 7144, cid, pcp.PCPHostFlags1Relay))
	good, _ := fakeYP(t, "503 Service Unavailable",
		testHostAtom(pcp.GnuID{3}, 0xCB007103, 7144, cid, pcp.PCPHostFlags1Tracker))
	yps := []string{closedPort(t), notFound, noTracker, good}
	addr, err := FindTracker(context.Background(), yps, cid, pcp.GnuID{9}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if addr != "203.0.113.3:7144" {
		t.Fatalf("tracker = %q", addr)
	}

	_, err = FindTracker(context.Background(), yps[:3], cid, pcp.GnuID{9}, 0)
	if !errors.Is(err, ErrTrackerNotFound) {
		t.Fatalf("err = %v, want ErrTrackerNotFound", err)
	}
	if _, err := FindTracker(context.Background(), nil, cid, pcp.GnuID{9}, 0); !errors.Is(err, ErrTrackerNotFound) {
		t.Fatalf("no YPs: err = %v, want ErrTrackerNotFound", err)
	}
}
