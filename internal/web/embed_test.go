package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func get(t *testing.T, h http.Handler, method, target string) *http.Response {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec.Result()
}

func body(t *testing.T, r *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestViewShellIdenticalForEveryID(t *testing.T) {
	h := Handler()
	a := get(t, h, "GET", "/s/q7Hk2xVw9aLm3ZpR4tYb8g")
	b := get(t, h, "GET", "/s/does-not-exist")
	if a.StatusCode != 200 || b.StatusCode != 200 {
		t.Fatalf("status %d / %d", a.StatusCode, b.StatusCode)
	}
	ba, bb := body(t, a), body(t, b)
	if ba != bb || ba == "" {
		t.Fatal("view shell differs between ids or is empty")
	}
	if strings.Contains(ba, "og:") {
		t.Fatal("view shell must not contain og: metadata")
	}
}

func TestHeadersAndTypes(t *testing.T) {
	h := Handler()
	for _, tc := range []struct{ path, ct string }{
		{"/", "text/html"},
		{"/s/abc", "text/html"},
		{"/static/style.css", "text/css"},
		{"/static/create.js", "text/javascript"},
		{"/static/vendor/qrcode.js", "text/javascript"},
	} {
		r := get(t, h, "GET", tc.path)
		if r.StatusCode != 200 {
			t.Fatalf("%s: status %d", tc.path, r.StatusCode)
		}
		if got := r.Header.Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s: Cache-Control = %q", tc.path, got)
		}
		if got := r.Header.Get("X-Robots-Tag"); !strings.Contains(got, "noindex") {
			t.Errorf("%s: X-Robots-Tag = %q", tc.path, got)
		}
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, tc.ct) {
			t.Errorf("%s: Content-Type = %q", tc.path, got)
		}
	}
}

func TestHeadAndNotFound(t *testing.T) {
	h := Handler()
	r := get(t, h, "HEAD", "/s/abc")
	if r.StatusCode != 200 || body(t, r) != "" {
		t.Fatalf("HEAD: status %d", r.StatusCode)
	}
	for _, p := range []string{"/static/", "/static/nope.js", "/static/vendor/", "/nope", "/s/", "/s/a/b"} {
		if r := get(t, h, "GET", p); r.StatusCode != 404 {
			t.Errorf("%s: status %d, want 404", p, r.StatusCode)
		}
	}
	// The mux cleans traversal paths with a redirect; either way it must not serve the file.
	if r := get(t, h, "GET", "/static/../view.html"); r.StatusCode == 200 {
		t.Errorf("traversal served with status 200")
	}
	if r := get(t, h, "POST", "/"); r.StatusCode != 405 {
		t.Errorf("POST /: status %d, want 405", r.StatusCode)
	}
}

func TestNoInlineScriptsOrStyles(t *testing.T) {
	h := Handler()
	for _, p := range []string{"/", "/s/x"} {
		b := body(t, get(t, h, "GET", p))
		for _, bad := range []string{" style=", "<style", "javascript:", "onclick=", "https://", "http://"} {
			if strings.Contains(b, bad) {
				t.Errorf("%s contains %q", p, bad)
			}
		}
		for _, s := range strings.Split(b, "<script")[1:] {
			if !strings.Contains(strings.SplitN(s, ">", 2)[0], "src=") {
				t.Errorf("%s has an inline script", p)
			}
		}
	}
}
