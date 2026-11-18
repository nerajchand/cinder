package store

import (
	"context"
	"sync"
	"time"
)

type memEntry struct {
	payload  Payload
	expires  time.Time
	consumed bool
}

// Memory is a mutex-based in-memory Store. It emulates the Redis semantics:
// atomic consume and a tombstone that lives until the original expiry.
// Intended for development, tests and single-instance use; data is lost on
// restart.
type Memory struct {
	mu      sync.Mutex
	entries map[string]*memEntry
	now     func() time.Time
}

// NewMemory returns an empty in-memory store.
func NewMemory() *Memory {
	return &Memory{entries: make(map[string]*memEntry), now: time.Now}
}

// live returns the entry for id if it has not expired, deleting it lazily
// otherwise. Caller must hold m.mu.
func (m *Memory) live(id string) *memEntry {
	e, ok := m.entries[id]
	if !ok {
		return nil
	}
	if !m.now().Before(e.expires) {
		delete(m.entries, id)
		return nil
	}
	return e
}

func clone(b []byte) []byte { return append([]byte(nil), b...) }

// Create implements Store.
func (m *Memory) Create(_ context.Context, id string, p Payload, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.live(id) != nil {
		return ErrIDCollision
	}
	m.entries[id] = &memEntry{
		payload: Payload{Ciphertext: clone(p.Ciphertext), IV: clone(p.IV)},
		expires: m.now().Add(ttl),
	}
	return nil
}

// Status implements Store.
func (m *Memory) Status(_ context.Context, id string) (State, time.Time, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.live(id)
	switch {
	case e == nil:
		return Gone, time.Time{}, nil
	case e.consumed:
		return Consumed, e.expires, nil
	default:
		return Available, e.expires, nil
	}
}

// Consume implements Store.
func (m *Memory) Consume(_ context.Context, id string) (Payload, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.live(id)
	if e == nil {
		return Payload{}, ErrNotFound
	}
	if e.consumed {
		return Payload{}, ErrConsumed
	}
	p := e.payload
	e.payload = Payload{} // burn: drop content, keep tombstone
	e.consumed = true
	return p, nil
}

// Ping implements Store.
func (m *Memory) Ping(context.Context) error { return nil }

// Sweep removes expired entries and returns how many were removed.
func (m *Memory) Sweep() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	now := m.now()
	for id, e := range m.entries {
		if !now.Before(e.expires) {
			delete(m.entries, id)
			n++
		}
	}
	return n
}

// RunJanitor sweeps expired entries every interval until ctx is done.
func (m *Memory) RunJanitor(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.Sweep()
		}
	}
}
