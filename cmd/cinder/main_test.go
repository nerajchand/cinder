package main

import (
	"net/http"
	"net/netip"
	"testing"
)

func TestUseMemory(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	cases := []struct {
		env  map[string]string
		want bool
	}{
		{map[string]string{}, true},
		{map[string]string{"CINDER_REDIS_URL": "redis://x:6379/0"}, false},
		{map[string]string{"CINDER_REDIS_URL": "redis://x:6379/0", "CINDER_STORE": "memory"}, true},
	}
	for i, c := range cases {
		if got := useMemory(env(c.env)); got != c.want {
			t.Errorf("case %d: got %v", i, got)
		}
	}
}

func TestSchemeFunc(t *testing.T) {
	f := schemeFunc([]netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")})
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
