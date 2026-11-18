// Package middleware provides the standard net/http middleware stack for
// Cinder: security headers, rate limiting, body limits, privacy-preserving
// access logging with panic recovery, and an Origin check for consume.
//
// All middlewares are plain func(http.Handler) http.Handler values.
package middleware

import (
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"

	"github.com/cinder/cinder/internal/config"
)

// Middleware wraps an http.Handler.
type Middleware = func(http.Handler) http.Handler

// Chain composes middlewares; the first argument is the outermost.
func Chain(mws ...Middleware) Middleware {
	return func(h http.Handler) http.Handler {
		for i := len(mws) - 1; i >= 0; i-- {
			if mws[i] != nil {
				h = mws[i](h)
			}
		}
		return h
	}
}

// Option customises New.
type Option func(*options)

type options struct {
	logger *slog.Logger
}

// WithLogger sets the access logger (default: discard).
func WithLogger(l *slog.Logger) Option { return func(o *options) { o.logger = l } }

// bodySlack covers JSON framing around the base64 fields.
const bodySlack = 1024

// MaxRequestBodyBytes returns the request body limit derived from the
// ciphertext limit: base64 expansion of ciphertext, a 12-byte IV (16 chars),
// plus JSON slack.
func MaxRequestBodyBytes(maxCiphertext int) int64 {
	b64 := int64(base64.RawURLEncoding.EncodedLen(maxCiphertext))
	return b64 + int64(base64.RawURLEncoding.EncodedLen(12)) + bodySlack
}

// New returns the standard stack, outermost first:
//
//	SecurityHeaders -> Logging -> Recover -> RateLimit -> BodyLimit
//
// Security headers are outermost so every response (including 429, 413, 500)
// carries them. Origin checking is route-specific; apply RequireOrigin to the
// consume handler.
func New(cfg config.Config, opts ...Option) Middleware {
	o := options{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	for _, opt := range opts {
		opt(&o)
	}
	return Chain(
		SecurityHeaders(cfg.TrustedProxies),
		Logging(o.logger),
		Recover(),
		RateLimit(cfg),
		BodyLimit(MaxRequestBodyBytes(cfg.MaxCiphertextBytes)),
	)
}
