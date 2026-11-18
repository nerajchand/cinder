package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestHealthURL(t *testing.T) {
	for addr, want := range map[string]string{
		"":               "http://127.0.0.1:8080/healthz",
		":9000":          "http://127.0.0.1:9000/healthz",
		"0.0.0.0:9000":   "http://127.0.0.1:9000/healthz",
		"127.0.0.1:9000": "http://127.0.0.1:9000/healthz",
	} {
		got, err := healthURL(func(string) string { return addr })
		if err != nil || got != want {
			t.Errorf("%q: got %q, %v", addr, got, err)
		}
	}
	if _, err := healthURL(func(string) string { return "nonsense" }); err == nil {
		t.Error("expected error")
	}
}

func TestHealthcheck(t *testing.T) {
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			w.WriteHeader(404)
			return
		}
		w.WriteHeader(status)
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	env := func(string) string { return u.Host }
	if err := healthcheck(env, srv.Client()); err != nil {
		t.Fatal(err)
	}
	status = http.StatusServiceUnavailable
	if err := healthcheck(env, srv.Client()); err == nil {
		t.Fatal("expected error on 503")
	}
}
