// Package integration holds end-to-end tests that boot the full handler stack
// (internal/app) over httptest, against the memory store and, when
// CINDER_TEST_REDIS_URL is set, a real Redis.
package integration

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/cinder/cinder/internal/app"
	"github.com/cinder/cinder/internal/config"
	"github.com/cinder/cinder/internal/store"
)

// syncBuf is a goroutine-safe log sink.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

type env struct {
	srv  *httptest.Server
	logs *syncBuf
	st   store.Store
}

func testConfig(t *testing.T) config.Config {
	t.Helper()
	cfg, err := config.Load(func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	// Tests hammer one client IP; the rate limiter has its own tests.
	cfg.RateLimitCreatePerMin = 1_000_000
	cfg.RateLimitConsumePerMin = 1_000_000
	cfg.RateLimitStatusPerMin = 1_000_000
	cfg.RateLimitBurst = 1_000_000
	return cfg
}

func newEnv(t *testing.T, st store.Store) *env {
	t.Helper()
	logs := &syncBuf{}
	log := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	srv := httptest.NewServer(app.NewHandler(testConfig(t), st, log))
	t.Cleanup(srv.Close)
	return &env{srv: srv, logs: logs, st: st}
}

func memEnv(t *testing.T) *env { return newEnv(t, store.NewMemory()) }

func redisEnv(t *testing.T) *env {
	t.Helper()
	url := os.Getenv("CINDER_TEST_REDIS_URL")
	if url == "" {
		t.Skip("CINDER_TEST_REDIS_URL not set")
	}
	r, err := store.NewRedis(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return newEnv(t, r)
}

var b64 = base64.RawURLEncoding

func (e *env) do(t *testing.T, method, path string, body string, hdr map[string]string) (int, http.Header, []byte) {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, e.srv.URL+path, rd)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, b
}

func (e *env) origin() map[string]string {
	return map[string]string{"Content-Type": "application/json", "Origin": e.srv.URL}
}

func (e *env) create(t *testing.T, ct []byte) (id, expires string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"ciphertext": b64.EncodeToString(ct),
		"iv":         b64.EncodeToString(make([]byte, 12)),
		"ttlSeconds": 3600,
	})
	code, _, b := e.do(t, "POST", "/api/secrets", string(body), map[string]string{"Content-Type": "application/json"})
	if code != 201 {
		t.Fatalf("create: %d %s", code, b)
	}
	var out struct{ ID, ExpiresAt string }
	if err := json.Unmarshal(b, &out); err != nil || out.ID == "" {
		t.Fatalf("create body %s", b)
	}
	return out.ID, out.ExpiresAt
}

func errCode(t *testing.T, b []byte) string {
	t.Helper()
	var out struct{ Error struct{ Code string } }
	_ = json.Unmarshal(b, &out)
	return out.Error.Code
}
