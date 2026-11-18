// Package idgen generates and validates secret ids.
package idgen

import (
	"crypto/rand"
	"encoding/base64"
)

const (
	// Bytes of entropy per id (128 bits minimum).
	Bytes = 16
	// Length of the encoded id (base64url, no padding).
	Length = 22
)

// New returns a fresh CSPRNG id: 16 random bytes, base64url without padding.
func New() (string, error) {
	var b [Bytes]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// Valid reports whether s is a well-formed id: exactly Length base64url
// characters that decode canonically to Bytes bytes.
func Valid(s string) bool {
	if len(s) != Length {
		return false
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(s)
	return err == nil && len(b) == Bytes
}
