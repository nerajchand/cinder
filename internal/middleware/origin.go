package middleware

import (
	"net/http"
	"net/netip"
	"strings"
)

// OriginChecker returns a predicate that accepts a request only if it has an
// Origin header equal to the request's own origin (scheme from TLS or trusted
// X-Forwarded-Proto, host from Host) or listed in allowed (normalised
// scheme://host[:port], as produced by internal/config). A missing or "null"
// Origin is rejected.
func OriginChecker(allowed []string, trusted []netip.Prefix) func(*http.Request) bool {
	set := make(map[string]struct{}, len(allowed))
	for _, a := range allowed {
		set[strings.ToLower(a)] = struct{}{}
	}
	return func(r *http.Request) bool {
		o := strings.ToLower(strings.TrimSpace(r.Header.Get("Origin")))
		if o == "" || o == "null" || len(r.Header.Values("Origin")) != 1 {
			return false
		}
		if _, ok := set[o]; ok {
			return true
		}
		scheme := "http"
		if isHTTPS(r, trusted) {
			scheme = "https"
		}
		return o == scheme+"://"+strings.ToLower(r.Host)
	}
}

// RequireOrigin rejects requests failing OriginChecker with 403 bad_origin.
// Apply it to the consume handler only.
func RequireOrigin(allowed []string, trusted []netip.Prefix) Middleware {
	ok := OriginChecker(allowed, trusted)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !ok(r) {
				writeError(w, http.StatusForbidden, "bad_origin", "Origin not allowed.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
