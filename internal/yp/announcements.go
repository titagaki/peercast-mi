package yp

import (
	"time"

	"github.com/titagaki/peercast-mi/internal/channel"
	"github.com/titagaki/peercast-mi/internal/pcputil"
	"github.com/titagaki/peercast-pcp/pcp"
)

type announcement struct {
	channel                                       *channel.Channel
	info                                          channel.ChannelInfo
	track                                         channel.TrackInfo
	receiving, relayFull, directFull, known, open bool
	global                                        string
	listeners, relays                             int
}

func (c *Client) sendAnnouncements(conn *pcp.Conn, force bool) error {
	if c.announced == nil {
		c.announced = make(map[pcp.GnuID]announcement)
	}
	next := make(map[pcp.GnuID]announcement)
	write := func(a *pcp.Atom) error {
		conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		defer conn.SetWriteDeadline(time.Time{})
		return conn.WriteAtom(a)
	}
	for _, ch := range c.mgr.List() {
		if !ch.IsBroadcasting() {
			continue
		}
		r, d := ch.SlotStatus(c.maxRelays, c.maxListeners)
		ip, known, open := c.Network.Status(c.localAddress)
		state := announcement{ch, ch.Info(), ch.Track(), ch.IsReceiving(), r, d, known, open, ip.String(), ch.TotalListeners(), ch.TotalRelays()}
		if prev, ok := c.announced[ch.ID]; force || !ok || prev != state {
			if err := write(c.buildBcst(ch)); err != nil {
				return err
			}
		}
		next[ch.ID] = state
	}
	for id, prev := range c.announced {
		if _, ok := next[id]; ok {
			continue
		}
		a := c.buildBcst(prev.channel)
		host := a.FindChild(pcp.PCPHost)
		flags := pcputil.Byte(host, pcp.PCPHostFlags1) &^ pcp.PCPHostFlags1Recv
		host = pcputil.ReplaceChild(host, pcp.NewByteAtom(pcp.PCPHostFlags1, flags))
		if err := write(pcputil.ReplaceChild(a, host)); err != nil {
			return err
		}
	}
	c.announced = next
	return nil
}
