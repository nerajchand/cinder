package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"
)

type ctxKey struct{}

// RequestID returns the request id set by Logging, or "".
func RequestID(ctx context.Context) string {
	s, _ := ctx.Value(ctxKey{}).(string)
	return s
}

func newRequestID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (s *statusWriter) WriteHeader(code int) {
	if !s.wrote {
		s.status, s.wrote = code, true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Write(b []byte) (int, error) {
	if !s.wrote {
		s.status, s.wrote = http.StatusOK, true
	}
	return s.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (s *statusWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// Logging emits one access-log line per request containing ONLY: method,
// route template, status, duration and a server-generated request id. It
// never logs paths, ids, query strings, bodies, IPs, User-Agent, Origin or
// Referer. Incoming X-Request-Id is ignored (the id is always generated); the
// id is returned in the X-Request-Id response header.
func Logging(l *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			id := newRequestID()
			w.Header().Set("X-Request-Id", id)
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			route, _ := classify(r)
			next.ServeHTTP(sw, r.WithContext(context.WithValue(r.Context(), ctxKey{}, id)))
			l.LogAttrs(r.Context(), slog.LevelInfo, "request",
				slog.String("method", r.Method),
				slog.String("route", route),
				slog.Int("status", sw.status),
				slog.Duration("duration", time.Since(start)),
				slog.String("request_id", id),
			)
		})
	}
}

// Recover converts panics into a generic 500 JSON error. The panic value is
// never logged or sent (it could contain request data).
func Recover() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if v := recover(); v != nil {
					if v == http.ErrAbortHandler {
						panic(v)
					}
					writeError(w, http.StatusInternalServerError, "internal", "Internal error.")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
