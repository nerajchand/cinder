// Package store defines the frozen storage contract for Cinder.
// The Redis implementation lives in a separate task; see docs/api.md for
// the key schema and atomic consume semantics.
package store

import (
	"context"
	"errors"
	"time"
)

// State is the externally visible state of a secret id.
type State int

const (
	// Gone means the id is unknown or expired (no secret, no tombstone).
	Gone State = iota
	// Available means the secret exists and has not been consumed.
	Available
	// Consumed means the secret was burned; a tombstone remains until the
	// original expiry.
	Consumed
)

func (s State) String() string {
	switch s {
	case Available:
		return "available"
	case Consumed:
		return "consumed"
	default:
		return "gone"
	}
}

// Payload is the opaque encrypted content. The server never sees plaintext.
type Payload struct {
	Ciphertext []byte
	IV         []byte
}

var (
	// ErrNotFound: no secret and no tombstone (never existed or expired).
	ErrNotFound = errors.New("store: not found")
	// ErrConsumed: the secret was already consumed (tombstone present).
	ErrConsumed = errors.New("store: already consumed")
	// ErrIDCollision: Create was called with an id that already exists
	// (secret or tombstone). Callers should generate a new id and retry.
	ErrIDCollision = errors.New("store: id collision")
)

// Store is the persistence contract. Implementations must be safe for
// concurrent use.
type Store interface {
	// Create stores p under id with the given ttl. Returns ErrIDCollision if
	// id already exists (as secret or tombstone).
	Create(ctx context.Context, id string, p Payload, ttl time.Duration) error
	// Status is non-destructive. It returns the state and the expiry time
	// (zero time when Gone). Gone is reported as state, not as an error.
	Status(ctx context.Context, id string) (State, time.Time, error)
	// Consume atomically reads and burns the secret, leaving a tombstone
	// until the original expiry. Returns ErrNotFound or ErrConsumed.
	// Exactly one concurrent caller may succeed.
	Consume(ctx context.Context, id string) (Payload, error)
	// Ping checks backend connectivity (used by /healthz).
	Ping(ctx context.Context) error
}
