package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/cinder/cinder/internal/idgen"
	"github.com/cinder/cinder/internal/store"
)

const (
	ivBytes        = 12
	maxCreateTries = 5
	storeTimeout   = 5 * time.Second
	bodySlack      = 1024
)

var ttlPresets = map[int64]bool{300: true, 3600: true, 86400: true, 604800: true}

var b64 = base64.RawURLEncoding.Strict()

type createRequest struct {
	Ciphertext string `json:"ciphertext"`
	IV         string `json:"iv"`
	TTLSeconds int64  `json:"ttlSeconds"`
}

type createResponse struct {
	ID        string `json:"id"`
	ExpiresAt string `json:"expiresAt"`
}

type statusResponse struct {
	State     string `json:"state"`
	ExpiresAt string `json:"expiresAt"`
}

type consumeResponse struct {
	Ciphertext string `json:"ciphertext"`
	IV         string `json:"iv"`
	ExpiresAt  string `json:"expiresAt,omitempty"`
}

func isJSON(r *http.Request) bool {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && mt == "application/json"
}

// decodeBody reads one JSON value of at most limit bytes into dst, rejecting
// unknown fields and trailing data. It writes the error response itself and
// returns false on failure.
func decodeBody(w http.ResponseWriter, r *http.Request, limit int64, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	err := dec.Decode(dst)
	if err == nil {
		// Reject trailing content after the first value.
		if _, terr := dec.Token(); terr != io.EOF {
			err = errors.New("trailing data")
		}
	}
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeError(w, http.StatusRequestEntityTooLarge, codePayloadTooLarge, "Request body too large.")
			return false
		}
		writeError(w, http.StatusBadRequest, codeBadRequest, "Invalid JSON body.")
		return false
	}
	return true
}

func (s *server) storeCtx(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), storeTimeout)
}

func (s *server) create(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errMethod(w, http.MethodPost)
		return
	}
	if !isJSON(r) {
		writeError(w, http.StatusUnsupportedMediaType, codeUnsupportedMedia, "Content-Type must be application/json.")
		return
	}
	// base64 expands 3 bytes to 4 chars; add room for iv, keys and slack.
	limit := int64(s.maxCT)*4/3 + 4 + 64 + bodySlack
	var req createRequest
	if !decodeBody(w, r, limit, &req) {
		return
	}
	if !ttlPresets[req.TTLSeconds] {
		writeError(w, http.StatusBadRequest, codeBadRequest, "ttlSeconds must be one of 300, 3600, 86400, 604800.")
		return
	}
	iv, err := b64.DecodeString(req.IV)
	if err != nil || len(iv) != ivBytes {
		writeError(w, http.StatusBadRequest, codeBadRequest, "iv must be 12 bytes, base64url without padding.")
		return
	}
	// Reject oversize before allocating the decoded buffer.
	if base64.RawURLEncoding.DecodedLen(len(req.Ciphertext)) > s.maxCT {
		writeError(w, http.StatusRequestEntityTooLarge, codePayloadTooLarge, "Ciphertext too large.")
		return
	}
	ct, err := b64.DecodeString(req.Ciphertext)
	if err != nil || len(ct) == 0 {
		writeError(w, http.StatusBadRequest, codeBadRequest, "ciphertext must be non-empty base64url without padding.")
		return
	}
	if len(ct) > s.maxCT {
		writeError(w, http.StatusRequestEntityTooLarge, codePayloadTooLarge, "Ciphertext too large.")
		return
	}

	ttl := time.Duration(req.TTLSeconds) * time.Second
	ctx, cancel := s.storeCtx(r)
	defer cancel()
	for i := 0; i < maxCreateTries; i++ {
		id, err := idgen.New()
		if err != nil {
			s.log.Error("id generation failed")
			errInternal(w)
			return
		}
		expires := s.now().Add(ttl).UTC()
		err = s.store.Create(ctx, id, store.Payload{Ciphertext: ct, IV: iv}, ttl)
		switch {
		case err == nil:
			writeJSON(w, http.StatusCreated, createResponse{ID: id, ExpiresAt: expires.Format(time.RFC3339)})
			return
		case errors.Is(err, store.ErrIDCollision):
			continue
		default:
			s.log.Error("store create failed", "err", err)
			errUnavailable(w)
			return
		}
	}
	s.log.Error("id collision retries exhausted")
	errInternal(w)
}

func (s *server) status(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		errMethod(w, "GET, HEAD")
		return
	}
	id := r.PathValue("id")
	if !idgen.Valid(id) {
		errNotFound(w)
		return
	}
	ctx, cancel := s.storeCtx(r)
	defer cancel()
	st, exp, err := s.store.Status(ctx, id)
	if err != nil {
		s.log.Error("store status failed", "err", err)
		errUnavailable(w)
		return
	}
	switch st {
	case store.Available:
		writeJSON(w, http.StatusOK, statusResponse{State: "available", ExpiresAt: exp.UTC().Format(time.RFC3339)})
	case store.Consumed:
		errAlreadyViewed(w)
	default:
		errNotFound(w)
	}
}

// originOK reports whether the Origin header matches the request origin or
// an allowed origin. A missing Origin is rejected.
func (s *server) originOK(r *http.Request) bool {
	o := strings.ToLower(r.Header.Get("Origin"))
	if o == "" {
		return false
	}
	if o == strings.ToLower(s.scheme(r)+"://"+r.Host) {
		return true
	}
	return s.allowedOrigins[o]
}

func (s *server) consume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errMethod(w, http.MethodPost)
		return
	}
	if !s.originOK(r) {
		writeError(w, http.StatusForbidden, codeBadOrigin, "Origin not allowed.")
		return
	}
	id := r.PathValue("id")
	if !idgen.Valid(id) {
		errNotFound(w)
		return
	}
	if !isJSON(r) {
		writeError(w, http.StatusUnsupportedMediaType, codeUnsupportedMedia, "Content-Type must be application/json.")
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64))
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeError(w, http.StatusRequestEntityTooLarge, codePayloadTooLarge, "Request body too large.")
			return
		}
		writeError(w, http.StatusBadRequest, codeBadRequest, "Invalid body.")
		return
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil || obj == nil || len(obj) != 0 {
		writeError(w, http.StatusBadRequest, codeBadRequest, "Body must be {}.")
		return
	}

	ctx, cancel := s.storeCtx(r)
	defer cancel()
	// expiresAt is not returned by Store.Consume; read it first (read-only).
	var exp time.Time
	if st, e, err := s.store.Status(ctx, id); err == nil && st == store.Available {
		exp = e
	}
	p, err := s.store.Consume(ctx, id)
	switch {
	case err == nil:
	case errors.Is(err, store.ErrConsumed):
		errAlreadyViewed(w)
		return
	case errors.Is(err, store.ErrNotFound):
		errNotFound(w)
		return
	default:
		s.log.Error("store consume failed", "err", err)
		errUnavailable(w)
		return
	}
	resp := consumeResponse{
		Ciphertext: base64.RawURLEncoding.EncodeToString(p.Ciphertext),
		IV:         base64.RawURLEncoding.EncodeToString(p.IV),
	}
	if !exp.IsZero() {
		resp.ExpiresAt = exp.UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, resp)
}
