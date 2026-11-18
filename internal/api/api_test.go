package api

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cinder/cinder/internal/idgen"
	"github.com/cinder/cinder/internal/store"
)

var b64e = base64.RawURLEncoding

// countingStore wraps a store and counts calls.
type countingStore struct {
	store.Store
	calls atomic.Int32
	err   error
}

func (c *countingStore) Create(ctx context.Context, id string, p store.Payload, ttl time.Duration) error {
	c.calls.Add(1)
	if c.err != nil {
		return c.err
	}
	return c.Store.Create(ctx, id, p, ttl)
}
func (c *countingStore) Status(ctx context.Context, id string) (store.State, time.Time, error) {
	c.calls.Add(1)
	if c.err != nil {
		return 0, time.Time{}, c.err
	}
	return c.Store.Status(ctx, id)
}
func (c *countingStore) Consume(ctx context.Context, id string) (store.Payload, error) {
	c.calls.Add(1)
	if c.err != nil {
		return store.Payload{}, c.err
	}
	return c.Store.Consume(ctx, id)
}
func (c *countingStore) Ping(ctx context.Context) error {
	if c.err != nil {
		return c.err
	}
	return c.Store.Ping(ctx)
}

func newTest(t *testing.T, opts ...func(*Deps)) (http.Handler, *countingStore) {
	t.Helper()
	cs := &countingStore{Store: store.NewMemory()}
	d := Deps{Store: cs, MaxCiphertextBytes: 1024}
	for _, o := range opts {
		o(&d)
	}
	web := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "shell") })
	return NewRouter(d, web), cs
}

func do(h http.Handler, method, path, body string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Host = "cinder.test"
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

var jsonHdr = map[string]string{"Content-Type": "application/json"}
var consumeHdr = map[string]string{"Content-Type": "application/json", "Origin": "http://cinder.test"}

func createBody(ct, iv string, ttl int) string {
	return fmt.Sprintf(`{"ciphertext":%q,"iv":%q,"ttlSeconds":%d}`, ct, iv, ttl)
}

func validIV() string { return b64e.EncodeToString(make([]byte, 12)) }

func mustCreate(t *testing.T, h http.Handler) string {
	t.Helper()
	rec := do(h, "POST", "/api/secrets", createBody(b64e.EncodeToString([]byte("hello-cipher")), validIV(), 3600), jsonHdr)
	if rec.Code != 201 {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	i := strings.Index(body, `"id":"`) + 6
	return body[i : i+22]
}

func wantCode(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, status, rec.Body)
	}
	if code != "" && !strings.Contains(rec.Body.String(), `"code":"`+code+`"`) {
		t.Fatalf("body %s lacks code %s", rec.Body, code)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("missing no-store")
	}
}

func TestCreateStatusConsumeFlow(t *testing.T) {
	h, _ := newTest(t)
	id := mustCreate(t, h)
	if !idgen.Valid(id) {
		t.Fatalf("bad id %q", id)
	}

	rec := do(h, "GET", "/api/secrets/"+id+"/status", "", nil)
	wantCode(t, rec, 200, "")
	if !strings.Contains(rec.Body.String(), `"state":"available"`) {
		t.Fatal(rec.Body)
	}

	rec = do(h, "POST", "/api/secrets/"+id+"/consume", "{}", consumeHdr)
	wantCode(t, rec, 200, "")
	if !strings.Contains(rec.Body.String(), b64e.EncodeToString([]byte("hello-cipher"))) ||
		!strings.Contains(rec.Body.String(), `"expiresAt"`) {
		t.Fatal(rec.Body)
	}

	wantCode(t, do(h, "POST", "/api/secrets/"+id+"/consume", "{}", consumeHdr), 410, "already_viewed")
	wantCode(t, do(h, "GET", "/api/secrets/"+id+"/status", "", nil), 410, "already_viewed")
}

func TestCreateValidation(t *testing.T) {
	h, _ := newTest(t)
	ct := b64e.EncodeToString([]byte("x"))
	big := b64e.EncodeToString(make([]byte, 1025))
	tests := []struct {
		name    string
		body    string
		hdr     map[string]string
		status  int
		code    string
		prePost bool
	}{
		{"bad ttl", createBody(ct, validIV(), 60), jsonHdr, 400, "bad_request", false},
		{"zero ttl", createBody(ct, validIV(), 0), jsonHdr, 400, "bad_request", false},
		{"short iv", createBody(ct, b64e.EncodeToString(make([]byte, 11)), 300), jsonHdr, 400, "bad_request", false},
		{"padded iv", createBody(ct, validIV()+"=", 300), jsonHdr, 400, "bad_request", false},
		{"std alphabet", createBody("+/+/", validIV(), 300), jsonHdr, 400, "bad_request", false},
		{"empty ct", createBody("", validIV(), 300), jsonHdr, 400, "bad_request", false},
		{"padded ct", createBody("YQ==", validIV(), 300), jsonHdr, 400, "bad_request", false},
		{"unknown field", `{"ciphertext":"YQ","iv":"` + validIV() + `","ttlSeconds":300,"x":1}`, jsonHdr, 400, "bad_request", false},
		{"not json", `nope`, jsonHdr, 400, "bad_request", false},
		{"trailing", createBody(ct, validIV(), 300) + `{}`, jsonHdr, 400, "bad_request", false},
		{"too big", createBody(big, validIV(), 300), jsonHdr, 413, "payload_too_large", false},
		{"huge body", `{"ciphertext":"` + strings.Repeat("A", 10000) + `"}`, jsonHdr, 413, "payload_too_large", false},
		{"wrong content type", createBody(ct, validIV(), 300), map[string]string{"Content-Type": "text/plain"}, 415, "unsupported_media_type", false},
		{"no content type", createBody(ct, validIV(), 300), nil, 415, "unsupported_media_type", false},
		{"json charset ok", createBody(ct, validIV(), 300), map[string]string{"Content-Type": "application/json; charset=utf-8"}, 201, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(h, "POST", "/api/secrets", tc.body, tc.hdr)
			if tc.status == 201 {
				if rec.Code != 201 {
					t.Fatalf("%d %s", rec.Code, rec.Body)
				}
				return
			}
			wantCode(t, rec, tc.status, tc.code)
		})
	}
	// exactly at the limit passes
	ok := b64e.EncodeToString(make([]byte, 1024))
	if rec := do(h, "POST", "/api/secrets", createBody(ok, validIV(), 300), jsonHdr); rec.Code != 201 {
		t.Fatalf("at-limit: %d %s", rec.Code, rec.Body)
	}
}

func TestCreateMethodNotAllowed(t *testing.T) {
	h, _ := newTest(t)
	rec := do(h, "GET", "/api/secrets", "", nil)
	wantCode(t, rec, 405, "method_not_allowed")
	if rec.Header().Get("Allow") != "POST" {
		t.Fatal("Allow")
	}
}

func TestCreateAllPresets(t *testing.T) {
	h, _ := newTest(t)
	for _, ttl := range []int{300, 3600, 86400, 604800} {
		rec := do(h, "POST", "/api/secrets", createBody("YQ", validIV(), ttl), jsonHdr)
		if rec.Code != 201 {
			t.Fatalf("ttl %d: %d", ttl, rec.Code)
		}
	}
}

type collideStore struct {
	store.Store
	n int
}

func (c *collideStore) Create(ctx context.Context, id string, p store.Payload, ttl time.Duration) error {
	c.n++
	if c.n < 3 {
		return store.ErrIDCollision
	}
	return c.Store.Create(ctx, id, p, ttl)
}

func TestCreateRetriesCollision(t *testing.T) {
	cs := &collideStore{Store: store.NewMemory()}
	h := NewRouter(Deps{Store: cs}, nil)
	if rec := do(h, "POST", "/api/secrets", createBody("YQ", validIV(), 300), jsonHdr); rec.Code != 201 {
		t.Fatalf("%d", rec.Code)
	}
	cs.n = -100
	if rec := do(h, "POST", "/api/secrets", createBody("YQ", validIV(), 300), jsonHdr); rec.Code != 500 {
		t.Fatalf("exhausted: %d", rec.Code)
	}
}

func TestStoreDown(t *testing.T) {
	h, cs := newTest(t)
	id := mustCreate(t, h)
	cs.err = errors.New("redis down")
	wantCode(t, do(h, "POST", "/api/secrets", createBody("YQ", validIV(), 300), jsonHdr), 503, "unavailable")
	wantCode(t, do(h, "GET", "/api/secrets/"+id+"/status", "", nil), 503, "unavailable")
	wantCode(t, do(h, "POST", "/api/secrets/"+id+"/consume", "{}", consumeHdr), 503, "unavailable")
	if rec := do(h, "GET", "/healthz", "", nil); rec.Code != 503 {
		t.Fatalf("healthz %d", rec.Code)
	}
}

func TestInvalidIDNoStoreCall(t *testing.T) {
	h, cs := newTest(t)
	for _, id := range []string{"short", strings.Repeat("A", 22) + "x", "..%2Fetc", strings.Repeat("!", 22), "AAAAAAAAAAAAAAAAAAAAAB"} {
		wantCode(t, do(h, "GET", "/api/secrets/"+id+"/status", "", nil), 404, "not_found")
		wantCode(t, do(h, "POST", "/api/secrets/"+id+"/consume", "{}", consumeHdr), 404, "not_found")
	}
	if n := cs.calls.Load(); n != 0 {
		t.Fatalf("store called %d times", n)
	}
}

func TestUnknownIDNotFound(t *testing.T) {
	h, _ := newTest(t)
	id, _ := idgen.New()
	wantCode(t, do(h, "GET", "/api/secrets/"+id+"/status", "", nil), 404, "not_found")
	wantCode(t, do(h, "POST", "/api/secrets/"+id+"/consume", "{}", consumeHdr), 404, "not_found")
}

func TestNonPostConsumeNeverBurns(t *testing.T) {
	h, _ := newTest(t)
	id := mustCreate(t, h)
	for _, m := range []string{"GET", "HEAD", "OPTIONS", "PUT", "DELETE", "PATCH"} {
		rec := do(h, m, "/api/secrets/"+id+"/consume", "{}", consumeHdr)
		if rec.Code != 405 || rec.Header().Get("Allow") != "POST" {
			t.Fatalf("%s: %d allow=%q", m, rec.Code, rec.Header().Get("Allow"))
		}
		if m != "HEAD" {
			wantCode(t, rec, 405, "method_not_allowed")
		}
	}
	// Also status via GET/HEAD/OPTIONS never burns.
	for _, m := range []string{"GET", "HEAD", "OPTIONS"} {
		do(h, m, "/api/secrets/"+id+"/status", "", nil)
	}
	for _, m := range []string{"GET", "HEAD", "OPTIONS"} {
		do(h, m, "/s/"+id, "", nil)
	}
	wantCode(t, do(h, "POST", "/api/secrets/"+id+"/consume", "{}", consumeHdr), 200, "")
}

func TestNonPostConsumeNoStoreCalls(t *testing.T) {
	h, cs := newTest(t)
	id := mustCreate(t, h)
	before := cs.calls.Load()
	for _, m := range []string{"GET", "HEAD", "OPTIONS", "PUT", "DELETE"} {
		do(h, m, "/api/secrets/"+id+"/consume", "", consumeHdr)
	}
	if cs.calls.Load() != before {
		t.Fatal("store touched by non-POST consume")
	}
}

func TestStatusHeadAndOptions(t *testing.T) {
	h, _ := newTest(t)
	id := mustCreate(t, h)
	if rec := do(h, "HEAD", "/api/secrets/"+id+"/status", "", nil); rec.Code != 200 {
		t.Fatalf("HEAD: %d", rec.Code)
	}
	wantCode(t, do(h, "OPTIONS", "/api/secrets/"+id+"/status", "", nil), 405, "method_not_allowed")
	wantCode(t, do(h, "POST", "/api/secrets/"+id+"/status", "", nil), 405, "method_not_allowed")
	// still available
	wantCode(t, do(h, "GET", "/api/secrets/"+id+"/status", "", nil), 200, "")
}

func TestConsumeOrigin(t *testing.T) {
	h, _ := newTest(t, func(d *Deps) { d.AllowedOrigins = []string{"https://alias.example"} })
	id := mustCreate(t, h)
	path := "/api/secrets/" + id + "/consume"
	bad := []map[string]string{
		{"Content-Type": "application/json"},
		{"Content-Type": "application/json", "Origin": "http://evil.test"},
		{"Content-Type": "application/json", "Origin": "https://cinder.test"}, // scheme mismatch
		{"Content-Type": "application/json", "Origin": "null"},
	}
	for _, hdr := range bad {
		wantCode(t, do(h, "POST", path, "{}", hdr), 403, "bad_origin")
	}
	// none of the above burned it
	wantCode(t, do(h, "GET", "/api/secrets/"+id+"/status", "", nil), 200, "")
	allowed := map[string]string{"Content-Type": "application/json", "Origin": "https://alias.example"}
	wantCode(t, do(h, "POST", path, "{}", allowed), 200, "")
}

func TestConsumeSchemeHook(t *testing.T) {
	h, _ := newTest(t, func(d *Deps) {
		d.RequestScheme = func(*http.Request) string { return "https" }
	})
	id := mustCreate(t, h)
	hdr := map[string]string{"Content-Type": "application/json", "Origin": "https://cinder.test"}
	wantCode(t, do(h, "POST", "/api/secrets/"+id+"/consume", "{}", hdr), 200, "")
}

func TestConsumeBodyAndContentType(t *testing.T) {
	h, _ := newTest(t)
	id := mustCreate(t, h)
	path := "/api/secrets/" + id + "/consume"
	wantCode(t, do(h, "POST", path, "{}", map[string]string{"Origin": "http://cinder.test"}), 415, "unsupported_media_type")
	wantCode(t, do(h, "POST", path, "{}", map[string]string{"Origin": "http://cinder.test", "Content-Type": "text/plain"}), 415, "")
	for _, b := range []string{"", "null", `{"a":1}`, "[]", "{} {}", "nope"} {
		wantCode(t, do(h, "POST", path, b, consumeHdr), 400, "bad_request")
	}
	wantCode(t, do(h, "POST", path, strings.Repeat(" ", 200)+"{}", consumeHdr), 413, "payload_too_large")
	// bad requests never burned it
	wantCode(t, do(h, "GET", "/api/secrets/"+id+"/status", "", nil), 200, "")
	wantCode(t, do(h, "POST", path, " {} \n", consumeHdr), 200, "")
}

func TestConsumeConcurrent(t *testing.T) {
	h, _ := newTest(t)
	id := mustCreate(t, h)
	var ok, gone, other atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			rec := do(h, "POST", "/api/secrets/"+id+"/consume", "{}", consumeHdr)
			switch rec.Code {
			case 200:
				ok.Add(1)
			case 410:
				gone.Add(1)
			default:
				other.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if ok.Load() != 1 || gone.Load() != 99 || other.Load() != 0 {
		t.Fatalf("200=%d 410=%d other=%d", ok.Load(), gone.Load(), other.Load())
	}
}

func TestHealthz(t *testing.T) {
	h, _ := newTest(t)
	rec := do(h, "GET", "/healthz", "", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

func TestWebMountAndUnknownAPI(t *testing.T) {
	h, cs := newTest(t)
	for _, p := range []string{"/", "/s/anything", "/static/app.js"} {
		if rec := do(h, "GET", p, "", nil); rec.Code != 200 || rec.Body.String() != "shell" {
			t.Fatalf("%s: %d", p, rec.Code)
		}
	}
	if cs.calls.Load() != 0 {
		t.Fatal("web touched store")
	}
	wantCode(t, do(h, "GET", "/api/nope", "", nil), 404, "not_found")
	if rec := do(h, "POST", "/s/abc", "", nil); rec.Code != 405 {
		t.Fatalf("POST /s: %d", rec.Code)
	}
}

func TestMiddlewareChainOrder(t *testing.T) {
	var order []string
	mk := func(name string) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name)
				next.ServeHTTP(w, r)
			})
		}
	}
	h := NewRouter(Deps{Store: store.NewMemory()}, nil, mk("a"), mk("b"))
	do(h, "GET", "/healthz", "", nil)
	if strings.Join(order, ",") != "a,b" {
		t.Fatalf("order %v", order)
	}
}
