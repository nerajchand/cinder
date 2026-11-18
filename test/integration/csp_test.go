package integration

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

var (
	scriptTag  = regexp.MustCompile(`(?is)<script\b([^>]*)>(.*?)</script>`)
	styleAttr  = regexp.MustCompile(`(?i)\sstyle\s*=`)
	inlineEvt  = regexp.MustCompile(`(?i)\son[a-z]+\s*=`)
	extURL     = regexp.MustCompile(`(?i)(?:https?:)?//[a-z0-9.-]+\.[a-z]{2,}|https?://`)
	linkedAttr = regexp.MustCompile(`(?i)\b(?:src|href)="(/[^"]*)"`)
)

// TestCSPCompatibility boots the full stack and checks that the middleware
// CSP and the shipped pages agree: nothing inline, nothing external.
func TestCSPCompatibility(t *testing.T) {
	e := memEnv(t)

	// Every response (even 404) carries the security header set.
	pages := []string{"/", "/s/x", "/s/q7Hk2xVw9aLm3ZpR4tYb8g"}
	assets := map[string]bool{}

	for _, p := range pages {
		code, h, b := e.do(t, "GET", p, "", nil)
		if code != 200 {
			t.Fatalf("%s: %d", p, code)
		}
		checkHeaders(t, p, h)
		if ct := h.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Errorf("%s content-type %q", p, ct)
		}
		html := string(b)
		for _, m := range scriptTag.FindAllStringSubmatch(html, -1) {
			if !strings.Contains(m[1], "src=") || strings.TrimSpace(m[2]) != "" {
				t.Errorf("%s: inline <script>: %q", p, m[0])
			}
		}
		if regexp.MustCompile(`(?i)<style\b`).MatchString(html) {
			t.Errorf("%s: <style> element", p)
		}
		if styleAttr.MatchString(html) {
			t.Errorf("%s: style attribute", p)
		}
		if inlineEvt.MatchString(html) {
			t.Errorf("%s: inline event handler", p)
		}
		if strings.Contains(strings.ToLower(html), "javascript:") {
			t.Errorf("%s: javascript: URL", p)
		}
		if m := extURL.FindString(html); m != "" {
			t.Errorf("%s: external URL %q", p, m)
		}
		for _, m := range linkedAttr.FindAllStringSubmatch(html, -1) {
			if strings.HasPrefix(m[1], "/static/") {
				assets[m[1]] = true
			}
		}
	}
	if len(assets) == 0 {
		t.Fatal("no /static assets referenced by pages")
	}
	// Also pull in assets the scripts import by name.
	for _, a := range []string{"/static/api.js", "/static/crypto.js", "/static/dom.js", "/static/render.js", "/static/vendor/qrcode.js", "/static/style.css", "/static/logo.svg"} {
		assets[a] = true
	}

	for a := range assets {
		code, h, b := e.do(t, "GET", a, "", nil)
		if code != 200 {
			t.Errorf("%s: %d", a, code)
			continue
		}
		checkHeaders(t, a, h)
		if h.Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: nosniff missing", a)
		}
		body := string(b)
		switch {
		case strings.HasSuffix(a, ".js"):
			if !strings.HasPrefix(h.Get("Content-Type"), "text/javascript") {
				t.Errorf("%s: content-type %q", a, h.Get("Content-Type"))
			}
			for _, bad := range []string{"eval(", "new Function", "toDataURL", "data:image", "innerHTML", "insertAdjacentHTML", "document.write", ".style.", "setAttribute('style'", `setAttribute("style"`} {
				if strings.Contains(body, bad) && !commentOnly(body, bad) {
					t.Errorf("%s: CSP-hostile construct %q", a, bad)
				}
			}
			for _, m := range regexp.MustCompile(`(?i)(?:fetch|import|WebSocket|EventSource|XMLHttpRequest|open)\s*\(\s*['"\x60]\s*(?:https?:|//|wss?:)`).FindAllString(body, -1) {
				t.Errorf("%s: cross-origin request %q", a, m)
			}
		case strings.HasSuffix(a, ".css"):
			if regexp.MustCompile(`(?i)@import|url\(\s*['"]?(?:https?:|//|data:)`).MatchString(body) {
				t.Errorf("%s: external/data url in CSS", a)
			}
		}
	}

	// canvas QR: the page has a canvas for the QR and style.css targets it;
	// img-src 'self' would block data: images, so the QR must be canvas-drawn.
	_, _, create := e.do(t, "GET", "/static/create.js", "", nil)
	if !strings.Contains(string(create), "qr-canvas") {
		t.Error("create.js does not draw QR to canvas")
	}
	_, _, idx := e.do(t, "GET", "/", "", nil)
	if !strings.Contains(string(idx), "<canvas") {
		t.Error("index.html has no canvas for the QR")
	}

	// API + 404 + 405 responses also carry headers.
	for _, c := range []struct{ m, p string }{{"GET", "/nope"}, {"GET", "/api/secrets/x/status"}, {"GET", "/healthz"}, {"DELETE", "/api/secrets"}} {
		_, h, _ := e.do(t, c.m, c.p, "", nil)
		checkHeaders(t, c.m+" "+c.p, h)
	}
}

func commentOnly(body, needle string) bool {
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, needle) {
			l := strings.TrimSpace(line)
			if !strings.HasPrefix(l, "//") && !strings.HasPrefix(l, "*") && !strings.HasPrefix(l, "/*") {
				return false
			}
		}
	}
	return true
}

func checkHeaders(t *testing.T, what string, h http.Header) {
	t.Helper()
	csp := h.Get("Content-Security-Policy")
	for _, want := range []string{"default-src 'none'", "script-src 'self'", "style-src 'self'", "connect-src 'self'", "img-src 'self'", "frame-ancestors 'none'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("%s: CSP missing %q: %s", what, want, csp)
		}
	}
	if strings.Contains(csp, "unsafe-") || strings.Contains(csp, "data:") {
		t.Errorf("%s: weak CSP %s", what, csp)
	}
	for k, v := range map[string]string{"Referrer-Policy": "no-referrer", "X-Content-Type-Options": "nosniff", "Cache-Control": "no-store"} {
		if h.Get(k) != v {
			t.Errorf("%s: %s = %q", what, k, h.Get(k))
		}
	}
}
