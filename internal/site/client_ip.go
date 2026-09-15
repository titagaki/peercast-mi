package site

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

func (s *Server) trustedProxy(ip netip.Addr) bool {
	for _, prefix := range s.trustedProxies {
		if prefix.Contains(ip.Unmap()) {
			return true
		}
	}
	return false
}

// Walk from the socket peer towards the client, stopping at the first
// untrusted address. Client-provided prefixes of X-Forwarded-For cannot win.
func (s *Server) broadcastClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return ""
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return ""
	}
	peer = peer.Unmap()
	if !s.trustedProxy(peer) {
		return peer.String()
	}
	chain := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(chain) - 1; i >= 0; i-- {
		if !s.trustedProxy(peer) {
			break
		}
		candidate, err := netip.ParseAddr(strings.TrimSpace(chain[i]))
		// Do not skip malformed hops and accidentally trust an earlier value.
		if err != nil {
			break
		}
		peer = candidate.Unmap()
	}
	return peer.String()
}
