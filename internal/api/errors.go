// Package api implements Cinder's HTTP API (see docs/api.md).
package api

import (
	"encoding/json"
	"net/http"
)

// Error codes from docs/api.md.
const (
	codeBadRequest       = "bad_request"
	codeBadOrigin        = "bad_origin"
	codeNotFound         = "not_found"
	codeMethodNotAllowed = "method_not_allowed"
	codeAlreadyViewed    = "already_viewed"
	codePayloadTooLarge  = "payload_too_large"
	codeUnsupportedMedia = "unsupported_media_type"
	codeUnavailable      = "unavailable"
	codeInternal         = "internal"
)

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// writeJSON writes v as JSON with the given status. Responses are always
// non-cacheable.
func writeJSON(w http.ResponseWriter, status int, v any) {
	h := w.Header()
	h.Set("Content-Type", "application/json")
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: msg}})
}

func errNotFound(w http.ResponseWriter) {
	writeError(w, http.StatusNotFound, codeNotFound, "Secret not found or expired.")
}

func errAlreadyViewed(w http.ResponseWriter) {
	writeError(w, http.StatusGone, codeAlreadyViewed, "This secret has already been viewed.")
}

func errUnavailable(w http.ResponseWriter) {
	writeError(w, http.StatusServiceUnavailable, codeUnavailable, "Storage is temporarily unavailable.")
}

func errInternal(w http.ResponseWriter) {
	writeError(w, http.StatusInternalServerError, codeInternal, "Internal error.")
}

func errMethod(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeError(w, http.StatusMethodNotAllowed, codeMethodNotAllowed, "Method not allowed.")
}
