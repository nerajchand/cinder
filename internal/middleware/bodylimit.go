package middleware

import (
	"errors"
	"net/http"
)

// BodyLimit caps request bodies at max bytes. A declared Content-Length over
// the limit is rejected immediately with 413 payload_too_large; otherwise the
// body is wrapped in http.MaxBytesReader (use IsBodyTooLarge on read errors).
func BodyLimit(max int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > max {
				writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "Payload too large.")
				return
			}
			if r.Body != nil && r.Body != http.NoBody {
				r.Body = http.MaxBytesReader(w, r.Body, max)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// IsBodyTooLarge reports whether err came from exceeding the body limit.
func IsBodyTooLarge(err error) bool {
	var mbe *http.MaxBytesError
	return errors.As(err, &mbe)
}

// WriteTooLarge writes the standard 413 error response.
func WriteTooLarge(w http.ResponseWriter) {
	writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "Payload too large.")
}
