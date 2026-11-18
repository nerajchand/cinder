package middleware

import (
	"net/http"
	"strings"
)

type routeClass int

const (
	classNone routeClass = iota
	classCreate
	classConsume
	classStatus
)

// classify maps a request path to a route template (safe to log: never
// contains a real id) and a rate-limit class. Anything unrecognised is
// reported as "other" so arbitrary paths are never logged.
func classify(r *http.Request) (string, routeClass) {
	p := r.URL.Path
	switch p {
	case "/":
		return "/", classNone
	case "/healthz":
		return "/healthz", classNone
	case "/api/secrets":
		return "/api/secrets", classCreate
	}
	if rest, ok := strings.CutPrefix(p, "/api/secrets/"); ok {
		if id, action, ok := strings.Cut(rest, "/"); ok && id != "" {
			switch action {
			case "status":
				return "/api/secrets/:id/status", classStatus
			case "consume":
				return "/api/secrets/:id/consume", classConsume
			}
		}
		return "other", classNone
	}
	if rest, ok := strings.CutPrefix(p, "/s/"); ok && rest != "" && !strings.Contains(rest, "/") {
		return "/s/:id", classNone
	}
	return "other", classNone
}
