package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestFlowMemory(t *testing.T) { runFlow(t, memEnv(t)) }
func TestFlowRedis(t *testing.T)  { runFlow(t, redisEnv(t)) }

func TestConcurrentConsumeMemory(t *testing.T) { runConcurrent(t, memEnv(t)) }
func TestConcurrentConsumeRedis(t *testing.T)  { runConcurrent(t, redisEnv(t)) }

func TestStreamedOversizeMemory(t *testing.T) { runStreamedOversize(t, memEnv(t)) }

func runFlow(t *testing.T, e *env) {
	ct := []byte("SUPER-SECRET-CIPHERTEXT-MARKER-0123456789")
	id, expires := e.create(t, ct)
	if expires == "" {
		t.Fatal("no expiresAt")
	}
	jsonHdr := map[string]string{"Content-Type": "application/json"}

	// Status is read-only, repeatable, and so are other GETs / HEAD / bad-origin POSTs.
	for i := 0; i < 3; i++ {
		code, _, b := e.do(t, "GET", "/api/secrets/"+id+"/status", "", nil)
		if code != 200 || !strings.Contains(string(b), `"available"`) {
			t.Fatalf("status %d %s", code, b)
		}
	}
	if code, _, _ := e.do(t, "GET", "/s/"+id, "", nil); code != 200 {
		t.Fatalf("view page %d", code)
	}
	if code, _, _ := e.do(t, "HEAD", "/api/secrets/"+id+"/status", "", nil); code != 200 {
		t.Fatalf("HEAD status %d", code)
	}
	if code, _, b := e.do(t, "GET", "/api/secrets/"+id+"/consume", "", nil); code != 405 {
		t.Fatalf("GET consume %d %s", code, b)
	}
	if code, _, b := e.do(t, "POST", "/api/secrets/"+id+"/consume", "{}", jsonHdr); code != 403 || errCode(t, b) != "bad_origin" {
		t.Fatalf("consume without Origin: %d %s", code, b)
	}
	bad := map[string]string{"Content-Type": "application/json", "Origin": "http://evil.example"}
	if code, _, _ := e.do(t, "POST", "/api/secrets/"+id+"/consume", "{}", bad); code != 403 {
		t.Fatalf("consume with foreign Origin: %d", code)
	}
	if code, _, _ := e.do(t, "GET", "/api/secrets/"+id+"/status", "", nil); code != 200 {
		t.Fatalf("secret was burned by non-consume requests: %d", code)
	}

	// Real consume.
	code, _, b := e.do(t, "POST", "/api/secrets/"+id+"/consume", "{}", e.origin())
	if code != 200 {
		t.Fatalf("consume %d %s", code, b)
	}
	var got struct{ Ciphertext, IV, ExpiresAt string }
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	dec, err := b64.DecodeString(got.Ciphertext)
	if err != nil || !bytes.Equal(dec, ct) {
		t.Fatalf("ciphertext mismatch: %q %v", dec, err)
	}
	if got.ExpiresAt != expires {
		t.Fatalf("expiresAt %q != create's %q", got.ExpiresAt, expires)
	}

	// Second consume and status: 410.
	code, _, b = e.do(t, "POST", "/api/secrets/"+id+"/consume", "{}", e.origin())
	if code != 410 || errCode(t, b) != "already_viewed" {
		t.Fatalf("second consume %d %s", code, b)
	}
	if code, _, b = e.do(t, "GET", "/api/secrets/"+id+"/status", "", nil); code != 410 {
		t.Fatalf("status after consume %d %s", code, b)
	}

	// Unknown but well-formed id: 404.
	code, _, b = e.do(t, "POST", "/api/secrets/AAAAAAAAAAAAAAAAAAAAAA/consume", "{}", e.origin())
	if code != 404 || errCode(t, b) != "not_found" {
		t.Fatalf("unknown consume %d %s", code, b)
	}
	if code, _, _ = e.do(t, "GET", "/api/secrets/AAAAAAAAAAAAAAAAAAAAAA/status", "", nil); code != 404 {
		t.Fatalf("unknown status %d", code)
	}

	// Healthz is up.
	if code, _, b = e.do(t, "GET", "/healthz", "", nil); code != 200 {
		t.Fatalf("healthz %d %s", code, b)
	}

	// Logs: no id, no ciphertext (raw or base64url), no ids of unknown probes.
	logs := e.logs.String()
	if logs == "" {
		t.Fatal("expected access logs")
	}
	for _, bad := range []string{id, string(ct), b64.EncodeToString(ct), "AAAAAAAAAAAAAAAAAAAAAA"} {
		if bad != "" && strings.Contains(logs, bad) {
			t.Fatalf("logs leak %q:\n%s", bad, logs)
		}
	}
}

func runConcurrent(t *testing.T, e *env) {
	id, _ := e.create(t, []byte("race-me"))
	const n = 200
	var ok, gone, other atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	// 200 goroutines, but cap simultaneous TCP dials: macOS' listen backlog
	// (~128) otherwise resets connections, which is not a server bug.
	client := &http.Client{Transport: &http.Transport{MaxConnsPerHost: 40}}
	defer client.CloseIdleConnections()
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			req, _ := http.NewRequest("POST", e.srv.URL+"/api/secrets/"+id+"/consume", strings.NewReader("{}"))
			for k, v := range e.origin() {
				req.Header.Set(k, v)
			}
			resp, err := client.Do(req)
			if err != nil {
				other.Add(1)
				t.Logf("client error: %v", err)
				return
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			switch resp.StatusCode {
			case 200:
				ok.Add(1)
			case 410:
				gone.Add(1)
			default:
				other.Add(1)
				t.Logf("unexpected status %d", resp.StatusCode)
			}
		}()
	}
	close(start)
	wg.Wait()
	if ok.Load() != 1 || gone.Load() != n-1 || other.Load() != 0 {
		t.Fatalf("200s=%d 410s=%d other=%d", ok.Load(), gone.Load(), other.Load())
	}
	if strings.Contains(e.logs.String(), id) {
		t.Fatal("logs contain id")
	}
}

// runStreamedOversize sends a chunked (no Content-Length) body larger than the
// limit; the create handler must answer with the 413 JSON error.
func runStreamedOversize(t *testing.T, e *env) {
	pr, pw := io.Pipe()
	go func() {
		chunk := bytes.Repeat([]byte("a"), 32<<10)
		_, _ = pw.Write([]byte(`{"ciphertext":"`))
		for i := 0; i < 8; i++ { // 256 KiB > 64 KiB limit
			if _, err := pw.Write(chunk); err != nil {
				return
			}
		}
		_, _ = pw.Write([]byte(`","iv":"","ttlSeconds":3600}`))
		pw.Close()
	}()
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/secrets", pr)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 413 || errCode(t, b) != "payload_too_large" {
		t.Fatalf("streamed oversize: %d %s", resp.StatusCode, b)
	}
	if resp.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("content-type %q", resp.Header.Get("Content-Type"))
	}
}
