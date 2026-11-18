package store

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var ctx = context.Background()

func TestMemoryLifecycle(t *testing.T) {
	m := NewMemory()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return now }

	if st, _, _ := m.Status(ctx, "a"); st != Gone {
		t.Fatalf("want Gone, got %v", st)
	}
	if err := m.Create(ctx, "a", Payload{Ciphertext: []byte("c"), IV: []byte("i")}, time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := m.Create(ctx, "a", Payload{}, time.Hour); err != ErrIDCollision {
		t.Fatalf("want collision, got %v", err)
	}
	st, exp, _ := m.Status(ctx, "a")
	if st != Available || !exp.Equal(now.Add(time.Hour)) {
		t.Fatalf("status = %v %v", st, exp)
	}
	p, err := m.Consume(ctx, "a")
	if err != nil || string(p.Ciphertext) != "c" || string(p.IV) != "i" {
		t.Fatalf("consume = %+v %v", p, err)
	}
	if _, err := m.Consume(ctx, "a"); err != ErrConsumed {
		t.Fatalf("want ErrConsumed, got %v", err)
	}
	if st, e2, _ := m.Status(ctx, "a"); st != Consumed || !e2.Equal(exp) {
		t.Fatalf("tombstone status = %v %v", st, e2)
	}
	// Tombstone blocks id reuse until original expiry.
	if err := m.Create(ctx, "a", Payload{}, time.Hour); err != ErrIDCollision {
		t.Fatalf("tombstone should collide, got %v", err)
	}
	now = now.Add(time.Hour)
	if st, _, _ := m.Status(ctx, "a"); st != Gone {
		t.Fatalf("want Gone after expiry, got %v", st)
	}
	if _, err := m.Consume(ctx, "a"); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestMemoryExpiryAndSweep(t *testing.T) {
	m := NewMemory()
	now := time.Now()
	m.now = func() time.Time { return now }
	_ = m.Create(ctx, "x", Payload{Ciphertext: []byte("c")}, time.Minute)
	now = now.Add(2 * time.Minute)
	if n := m.Sweep(); n != 1 {
		t.Fatalf("swept %d", n)
	}
	if err := m.Create(ctx, "x", Payload{}, time.Minute); err != nil {
		t.Fatalf("recreate after expiry: %v", err)
	}
}

func TestMemoryConsumeConcurrent(t *testing.T) {
	m := NewMemory()
	_ = m.Create(ctx, "id", Payload{Ciphertext: []byte("c"), IV: []byte("i")}, time.Hour)
	var ok, consumed int32
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			switch _, err := m.Consume(ctx, "id"); err {
			case nil:
				atomic.AddInt32(&ok, 1)
			case ErrConsumed:
				atomic.AddInt32(&consumed, 1)
			}
		}()
	}
	wg.Wait()
	if ok != 1 || consumed != 199 {
		t.Fatalf("ok=%d consumed=%d", ok, consumed)
	}
}

func TestMemoryCopiesInput(t *testing.T) {
	m := NewMemory()
	c := []byte("abc")
	_ = m.Create(ctx, "id", Payload{Ciphertext: c, IV: []byte("i")}, time.Hour)
	c[0] = 'z'
	p, _ := m.Consume(ctx, "id")
	if string(p.Ciphertext) != "abc" {
		t.Fatalf("store aliased caller buffer: %q", p.Ciphertext)
	}
}

var _ Store = (*Memory)(nil)
