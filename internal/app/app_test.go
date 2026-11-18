package app

import (
	"net/http"
	"net/netip"
	"testing"
)

func TestSchemeFunc(t *testing.T) {
	f := SchemeFunc([]netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")})
	mk := func(remote, xfp string) *http.Request {
		r, _ := http.NewRequest("GET", "/", nil)
		r.RemoteAddr = remote
		if xfp != "" {
			r.Header.Set("X-Forwarded-Proto", xfp)
		}
		return r
	}
	if f(mk("10.1.2.3:1234", "https")) != "https" {
		t.Error("trusted proxy should be honoured")
	}
	if f(mk("8.8.8.8:1234", "https")) != "http" {
		t.Error("untrusted proxy must be ignored")
	}
	if f(mk("10.1.2.3:1234", "")) != "http" {
		t.Error("no header -> http")
	}
}
