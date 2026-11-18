package middleware

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cinder/cinder/internal/config"
)

func baseCfg() config.Config {
	return config.Config{
		MaxCiphertextBytes:     1024,
		RateLimitCreatePerMin:  60,
		RateLimitConsumePerMin: 60,
		RateLimitStatusPerMin:  60,
		RateLimitBurst:         2,
	}
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/missing":
			http.NotFound(w, r)
		case r.URL.Path == "/api/secrets/abc/consume" && r.Method != http.MethodPost:
			w.Header().Set("Allow", "POST")
			http.Error(w, "no", http.StatusMethodNotAllowed)
		default:
			w.WriteHeader(http.StatusOK)
		}
	})
}

func req(method, target, remote string) *http.Request {
	r := httptest.NewRequest(method, target, nil)
	r.RemoteAddr = remote
	return r
}

func TestSecurityHeadersTable(t *testing.T) {
	cfg := baseCfg()
	cfg.RateLimitStatusPerMin = 0 // enable only create for 429 below
	cfg.RateLimitCreatePerMin = 1
	cfg.RateLimitBurst = 1
	h := New(cfg)(okHandler())
	// Exhaust create bucket for 429.
	h.ServeHTTP(httptest.NewRecorder(), req("POST", "/api/secrets", "9.9.9.9:1"))

	big := httptest.NewRequest("POST", "/api/secrets/x", strings.NewReader(strings.Repeat("a", 100000)))
	big.RemoteAddr = "8.8.8.8:1"
	big.ContentLength = 100000

	cases := []struct {
		name string
		r    *http.Request
		want int
	}{
		{"200", req("GET", "/s/abc", "1.1.1.1:1"), 200},
		{"404", req("GET", "/missing", "1.1.1.1:1"), 404},
		{"405", req("GET", "/api/secrets/abc/consume", "1.1.1.1:1"), 405},
		{"429", req("POST", "/api/secrets", "9.9.9.9:1"), 429},
		{"413", big, 413},
	}
	want := map[string]string{
		"Content-Security-Policy":      csp,
		"Referrer-Policy":              "no-referrer",
		"Cache-Control":                "no-store",
		"X-Content-Type-Options":       "nosniff",
		"Cross-Origin-Opener-Policy":   "same-origin",
		"Cross-Origin-Resource-Policy": "same-origin",
		"X-Robots-Tag":                 "noindex, nofollow",
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, c.r)
			if rec.Code != c.want {
				t.Fatalf("status %d want %d", rec.Code, c.want)
			}
			for k, v := range want {
				if got := rec.Header().Get(k); got != v {
					t.Errorf("%s = %q want %q", k, got, v)
				}
			}
			if rec.Header().Get("Permissions-Policy") == "" {
				t.Error("missing Permissions-Policy")
			}
			if rec.Header().Get("Strict-Transport-Security") != "" {
				t.Error("HSTS on plain http")
			}
			if strings.Contains(csp, "unsafe-") || strings.Contains(csp, "data:") {
				t.Error("weak CSP")
			}
		})
	}
}

func TestHSTS(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	h := SecurityHeaders(trusted)(okHandler())
	cases := []struct {
		name   string
		remote string
		proto  string
		tls    bool
		want   bool
	}{
		{"plain", "1.1.1.1:1", "", false, false},
		{"tls", "1.1.1.1:1", "", true, true},
		{"untrusted xfp", "1.1.1.1:1", "https", false, false},
		{"trusted xfp https", "10.1.1.1:1", "https", false, true},
		{"trusted xfp http", "10.1.1.1:1", "http", false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := req("GET", "/", c.remote)
			if c.proto != "" {
				r.Header.Set("X-Forwarded-Proto", c.proto)
			}
			if c.tls {
				r.TLS = &tlsState
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, r)
			if got := rec.Header().Get("Strict-Transport-Security") != ""; got != c.want {
				t.Fatalf("hsts=%v want %v", got, c.want)
			}
		})
	}
}

type fakeClock struct{ t time.Time }

func (f *fakeClock) now() time.Time { return f.t }

func serve(h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func TestRateLimit(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	cfg := baseCfg() // 1 token/s, burst 2
	h := rateLimit(cfg, clk.now)(okHandler())
	do := func(remote string) *httptest.ResponseRecorder {
		return serve(h, req("GET", "/api/secrets/abc/status", remote))
	}

	t.Run("burst", func(t *testing.T) {
		for i := 0; i < 2; i++ {
			if c := do("1.1.1.1:1").Code; c != 200 {
				t.Fatalf("req %d: %d", i, c)
			}
		}
		rec := do("1.1.1.1:1")
		if rec.Code != 429 {
			t.Fatalf("got %d", rec.Code)
		}
		if ra, _ := strconv.Atoi(rec.Header().Get("Retry-After")); ra < 1 {
			t.Fatalf("Retry-After %q", rec.Header().Get("Retry-After"))
		}
		var body struct {
			Error struct{ Code, Message string }
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Error.Code != "rate_limited" {
			t.Fatalf("body %s", rec.Body)
		}
		if rec.Header().Get("Content-Type") != "application/json" {
			t.Fatal("content type")
		}
	})
	t.Run("refill", func(t *testing.T) {
		clk.t = clk.t.Add(1100 * time.Millisecond)
		if c := do("1.1.1.1:1").Code; c != 200 {
			t.Fatalf("after refill %d", c)
		}
		if c := do("1.1.1.1:1").Code; c != 429 {
			t.Fatalf("expected 429 got %d", c)
		}
	})
	t.Run("per-IP isolation", func(t *testing.T) {
		if c := do("2.2.2.2:1").Code; c != 200 {
			t.Fatalf("other ip %d", c)
		}
	})
	t.Run("unlimited class", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			if c := serve(h, req("GET", "/s/abc", "1.1.1.1:1")).Code; c != 200 {
				t.Fatalf("/s limited: %d", c)
			}
		}
	})
	t.Run("classes independent", func(t *testing.T) {
		for i := 0; i < 2; i++ {
			serve(h, req("POST", "/api/secrets", "3.3.3.3:1"))
		}
		if c := serve(h, req("POST", "/api/secrets", "3.3.3.3:1")).Code; c != 429 {
			t.Fatalf("create %d", c)
		}
		if c := serve(h, req("POST", "/api/secrets/abc/consume", "3.3.3.3:1")).Code; c != 200 {
			t.Fatalf("consume %d", c)
		}
	})
}

func TestRateLimitDisabledAndStricter(t *testing.T) {
	cfg := baseCfg()
	cfg.RateLimitStatusPerMin = 0
	h := rateLimit(cfg, time.Now)(okHandler())
	for i := 0; i < 50; i++ {
		if c := serve(h, req("GET", "/api/secrets/abc/status", "1.1.1.1:1")).Code; c != 200 {
			t.Fatalf("disabled class limited: %d", c)
		}
	}
}

func TestForwardedFor(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	cfg := baseCfg()
	cfg.RateLimitBurst = 1
	cfg.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	h := rateLimit(cfg, clk.now)(okHandler())
	do := func(remote, xff string) int {
		r := req("GET", "/api/secrets/abc/status", remote)
		if xff != "" {
			r.Header.Set("X-Forwarded-For", xff)
		}
		return serve(h, r).Code
	}
	// Untrusted peer: XFF ignored, spoofing does not evade the limit.
	if do("1.1.1.1:1", "5.5.5.5") != 200 || do("1.1.1.1:1", "6.6.6.6") != 429 {
		t.Fatal("XFF honoured from untrusted peer")
	}
	// Trusted peer: XFF used; distinct clients isolated.
	if do("10.0.0.1:1", "7.7.7.7") != 200 || do("10.0.0.1:1", "8.8.8.8") != 200 {
		t.Fatal("trusted XFF clients not isolated")
	}
	if do("10.0.0.1:1", "7.7.7.7") != 429 {
		t.Fatal("trusted XFF client not limited")
	}
	// Client-supplied leftmost entries are not trusted: rightmost untrusted wins.
	if do("10.0.0.1:1", "4.4.4.4, 7.7.7.7, 10.0.0.9") != 429 {
		t.Fatal("rightmost-untrusted not used")
	}
	// Trusted peer without XFF: bucket is the proxy.
	if do("10.0.0.2:1", "") != 200 || do("10.0.0.2:1", "") != 429 {
		t.Fatal("fallback to peer")
	}
}

func TestLimiterBounded(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	l := newLimiter(60, 2, clk.now)
	for i := 0; i < 5; i++ {
		l.allow(netip.AddrFrom4([4]byte{10, 0, 0, byte(i)}))
	}
	if l.size() != 5 {
		t.Fatal(l.size())
	}
	clk.t = clk.t.Add(2 * sweepInterval)
	l.allow(netip.MustParseAddr("1.2.3.4"))
	if l.size() != 1 {
		t.Fatalf("idle buckets not swept: %d", l.size())
	}
	// Hard cap.
	l = newLimiter(60, 2, clk.now)
	for i := 0; i < maxBuckets+50; i++ {
		l.allow(netip.AddrFrom4([4]byte{byte(i >> 16), byte(i >> 8), byte(i), 1}))
	}
	if l.size() > maxBuckets {
		t.Fatalf("exceeded cap: %d", l.size())
	}
}

func TestBodyLimit(t *testing.T) {
	h := BodyLimit(10)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 100)
		_, err := r.Body.Read(buf)
		if err != nil && IsBodyTooLarge(err) {
			WriteTooLarge(w)
			return
		}
		w.WriteHeader(200)
	}))
	// Chunked (unknown length) over limit hits MaxBytesReader.
	r := httptest.NewRequest("POST", "/", strings.NewReader(strings.Repeat("a", 50)))
	r.ContentLength = -1
	if c := serve(h, r).Code; c != 413 {
		t.Fatalf("chunked: %d", c)
	}
	r = httptest.NewRequest("POST", "/", strings.NewReader(strings.Repeat("a", 50)))
	if c := serve(h, r).Code; c != 413 {
		t.Fatalf("declared: %d", c)
	}
	r = httptest.NewRequest("POST", "/", strings.NewReader("small"))
	if c := serve(h, r).Code; c != 200 {
		t.Fatalf("small: %d", c)
	}
	if MaxRequestBodyBytes(65536) < 87382+16 {
		t.Fatal("body limit too small for base64 ciphertext")
	}
}

func TestOrigin(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	chk := OriginChecker([]string{"https://alt.example"}, trusted)
	cases := []struct {
		name, remote, host, origin, xfp string
		want                            bool
	}{
		{"missing", "1.1.1.1:1", "cinder.test", "", "", false},
		{"null", "1.1.1.1:1", "cinder.test", "null", "", false},
		{"same http", "1.1.1.1:1", "cinder.test", "http://cinder.test", "", true},
		{"scheme mismatch", "1.1.1.1:1", "cinder.test", "https://cinder.test", "", false},
		{"untrusted xfp ignored", "1.1.1.1:1", "cinder.test", "https://cinder.test", "https", false},
		{"trusted xfp https", "10.0.0.1:1", "cinder.test", "https://cinder.test", "https", true},
		{"other host", "1.1.1.1:1", "cinder.test", "http://evil.test", "", false},
		{"allowlisted", "1.1.1.1:1", "cinder.test", "https://alt.example", "", true},
		{"case", "1.1.1.1:1", "Cinder.Test", "http://cinder.TEST", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := req("POST", "/api/secrets/abc/consume", c.remote)
			r.Host = c.host
			if c.origin != "" {
				r.Header.Set("Origin", c.origin)
			}
			if c.xfp != "" {
				r.Header.Set("X-Forwarded-Proto", c.xfp)
			}
			if got := chk(r); got != c.want {
				t.Fatalf("got %v", got)
			}
		})
	}
	rec := serve(RequireOrigin(nil, nil)(okHandler()), req("POST", "/x", "1.1.1.1:1"))
	if rec.Code != 403 || !strings.Contains(rec.Body.String(), `"bad_origin"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

func TestRecover(t *testing.T) {
	var buf bytes.Buffer
	l := slog.New(slog.NewTextHandler(&buf, nil))
	h := New(baseCfg(), WithLogger(l))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("secret-panic-value")
	}))
	rec := serve(h, req("GET", "/s/abc", "1.1.1.1:1"))
	if rec.Code != 500 || !strings.Contains(rec.Body.String(), `"internal"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "secret-panic-value") || strings.Contains(buf.String(), "secret-panic-value") {
		t.Fatal("panic value leaked")
	}
	if rec.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("headers missing on 500")
	}
	if !strings.Contains(buf.String(), "status=500") {
		t.Fatalf("500 not logged: %s", buf.String())
	}
}

func TestLogsNeverContainSensitive(t *testing.T) {
	const (
		id     = "AAAAAAAAAAAAAAAAAAAAAA"
		cipher = "CANARYCIPHERTEXTzzzzzzzz"
		query  = "CANARYQUERYtoken"
		origin = "https://canary-origin.example"
		ua     = "CANARY-UA/1.0"
	)
	var buf bytes.Buffer
	l := slog.New(slog.NewJSONHandler(&buf, nil))
	cfg := baseCfg()
	cfg.RateLimitBurst = 1
	h := New(cfg, WithLogger(l))(okHandler())

	targets := []struct{ method, path string }{
		{"POST", "/api/secrets/" + id + "/consume?k=" + query},
		{"GET", "/api/secrets/" + id + "/status?k=" + query},
		{"GET", "/api/secrets/" + id + "/status?k=" + query}, // 429
		{"GET", "/s/" + id + "?k=" + query + "#frag"},
		{"POST", "/api/secrets?k=" + query},
		{"GET", "/totally/unknown/" + id + "/path?k=" + query},
		{"GET", "/s/" + id + "/extra"},
	}
	for _, tc := range targets {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{"ciphertext":"`+cipher+`"}`))
		r.RemoteAddr = "203.0.113.77:4444"
		r.Header.Set("Origin", origin)
		r.Header.Set("Referer", origin+"/s/"+id)
		r.Header.Set("User-Agent", ua)
		r.Header.Set("X-Request-Id", "CANARY-INCOMING-RID")
		serve(h, r)
	}
	out := buf.String()
	for _, bad := range []string{id, cipher, query, "canary-origin", ua, "203.0.113.77", "CANARY-INCOMING-RID", "frag", "unknown", "extra", "?"} {
		if strings.Contains(out, bad) {
			t.Errorf("log contains %q:\n%s", bad, out)
		}
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != len(targets) {
		t.Fatalf("lines %d want %d", len(lines), len(targets))
	}
	for _, ln := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(ln), &m); err != nil {
			t.Fatal(err)
		}
		for k := range m {
			switch k {
			case "time", "level", "msg", "method", "route", "status", "duration", "request_id":
			default:
				t.Errorf("unexpected log field %q", k)
			}
		}
	}
	if !strings.Contains(out, "/api/secrets/:id/consume") || !strings.Contains(out, `"status":429`) {
		t.Errorf("expected template and 429 in log:\n%s", out)
	}
}

func TestChainOrder(t *testing.T) {
	var order []string
	mk := func(n string) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, n)
				next.ServeHTTP(w, r)
			})
		}
	}
	serve(Chain(mk("a"), nil, mk("b"))(okHandler()), req("GET", "/", "1.1.1.1:1"))
	if strings.Join(order, "") != "ab" {
		t.Fatal(order)
	}
}

var tlsState = tls.ConnectionState{}
