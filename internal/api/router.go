package api

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/cinder/cinder/internal/store"
)

// Middleware wraps a handler (provided by internal/middleware).
type Middleware func(http.Handler) http.Handler

// Deps are the router's dependencies.
type Deps struct {
	Store store.Store
	// MaxCiphertextBytes bounds the decoded ciphertext size.
	MaxCiphertextBytes int
	// AllowedOrigins are extra normalized origins (scheme://host[:port])
	// accepted on consume, besides the request's own origin.
	AllowedOrigins []string
	// OriginCheck, when set, is THE authoritative consume Origin check (the
	// handler runs it before anything else; no other layer repeats it).
	// cmd/cinder passes middleware.OriginChecker. When nil, a built-in
	// check using AllowedOrigins and RequestScheme is used.
	OriginCheck func(*http.Request) bool
	// RequestScheme reports the scheme ("http"/"https") as seen by the
	// client, honouring trusted proxies. Defaults to TLS detection.
	RequestScheme func(*http.Request) string
	// Logger must never receive secret material. Defaults to slog.Default().
	Logger *slog.Logger
	// Now is overridable for tests.
	Now func() time.Time
}

type server struct {
	store          store.Store
	maxCT          int
	allowedOrigins map[string]bool
	scheme         func(*http.Request) string
	originCheck    func(*http.Request) bool
	log            *slog.Logger
	now            func() time.Time
}

// NewRouter builds the HTTP handler. web serves the static shell (GET /,
// /s/{id}, /static/) and may be nil. chain is applied outermost-first around
// the whole mux (first element is outermost).
func NewRouter(d Deps, web http.Handler, chain ...Middleware) http.Handler {
	s := &server{
		store:          d.Store,
		maxCT:          d.MaxCiphertextBytes,
		allowedOrigins: map[string]bool{},
		scheme:         d.RequestScheme,
		originCheck:    d.OriginCheck,
		log:            d.Logger,
		now:            d.Now,
	}
	if s.maxCT <= 0 {
		s.maxCT = 64 * 1024
	}
	for _, o := range d.AllowedOrigins {
		s.allowedOrigins[strings.ToLower(o)] = true
	}
	if s.scheme == nil {
		s.scheme = func(r *http.Request) string {
			if r.TLS != nil {
				return "https"
			}
			return "http"
		}
	}
	if s.log == nil {
		s.log = slog.Default()
	}
	if s.now == nil {
		s.now = time.Now
	}

	mux := http.NewServeMux()
	// Method checks happen inside handlers so that 405s use the JSON error
	// shape and never reach storage.
	mux.HandleFunc("/api/secrets", s.create)
	mux.HandleFunc("/api/secrets/{id}/status", s.status)
	mux.HandleFunc("/api/secrets/{id}/consume", s.consume)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, codeNotFound, "Not found.")
	})
	mux.HandleFunc("GET /healthz", s.health)
	if web != nil {
		mux.Handle("GET /{$}", web)
		mux.Handle("GET /s/{id}", web)
		mux.Handle("GET /static/", web)
	}

	var h http.Handler = mux
	for i := len(chain) - 1; i >= 0; i-- {
		h = chain[i](h)
	}
	return h
}
