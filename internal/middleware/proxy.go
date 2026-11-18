package middleware

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

func peerAddr(r *http.Request) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	a, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	return a.Unmap(), true
}

func isTrusted(a netip.Addr, trusted []netip.Prefix) bool {
	for _, p := range trusted {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// peerTrusted reports whether the direct TCP peer is a trusted proxy.
func peerTrusted(r *http.Request, trusted []netip.Prefix) bool {
	a, ok := peerAddr(r)
	return ok && isTrusted(a, trusted)
}

// clientIP returns the client address. X-Forwarded-For is consulted only
// when the direct peer is a trusted proxy; it is walked right to left and
// the first address not itself a trusted proxy wins.
func clientIP(r *http.Request, trusted []netip.Prefix) (netip.Addr, bool) {
	peer, ok := peerAddr(r)
	if !ok {
		return netip.Addr{}, false
	}
	if !isTrusted(peer, trusted) {
		return peer, true
	}
	parts := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(parts) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(strings.TrimSpace(parts[i]))
		if err != nil {
			return peer, true // malformed chain: fall back to the proxy itself
		}
		a = a.Unmap()
		if !isTrusted(a, trusted) {
			return a, true
		}
	}
	return peer, true
}

// isHTTPS reports whether the request arrived over TLS, or a trusted proxy
// says X-Forwarded-Proto=https.
func isHTTPS(r *http.Request, trusted []netip.Prefix) bool {
	if r.TLS != nil {
		return true
	}
	if !peerTrusted(r, trusted) {
		return false
	}
	v := r.Header.Get("X-Forwarded-Proto")
	if i := strings.IndexByte(v, ','); i >= 0 {
		v = v[:i]
	}
	return strings.EqualFold(strings.TrimSpace(v), "https")
}
