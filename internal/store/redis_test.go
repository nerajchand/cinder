package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func newTestStore(t *testing.T) *Redis {
	t.Helper()
	url := os.Getenv("CINDER_TEST_REDIS_URL")
	if url == "" {
		c, err := net.DialTimeout("tcp", "127.0.0.1:6379", 200*time.Millisecond)
		if err != nil {
			t.Skip("CINDER_TEST_REDIS_URL unset and no Redis on 127.0.0.1:6379")
		}
		_ = c.Close()
		url = "redis://127.0.0.1:6379/15"
	}
	ctx := context.Background()
	s, err := NewRedis(ctx, url)
	if err != nil {
		t.Fatalf("NewRedis: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

var testSeq atomic.Int64

func newID(t *testing.T, s *Redis) string {
	id := fmt.Sprintf("%s-%d-%d", t.Name(), time.Now().UnixNano(), testSeq.Add(1))
	t.Cleanup(func() { s.rdb.Del(context.Background(), secretPrefix+id, tombPrefix+id) })
	return id
}

func payload() Payload {
	// include bytes that are awkward for text handling
	return Payload{IV: []byte("\x00\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0a\xff"), Ciphertext: []byte("\x00ct\r\n\xff\xfe binary")}
}

func TestCreateConsumeRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	id := newID(t, s)
	p := payload()
	if err := s.Create(ctx, id, p, time.Hour); err != nil {
		t.Fatal(err)
	}
	st, exp, err := s.Status(ctx, id)
	if err != nil || st != Available {
		t.Fatalf("status = %v, %v", st, err)
	}
	if d := time.Until(exp); d < 59*time.Minute || d > time.Hour {
		t.Fatalf("expiry off: %v", d)
	}
	got, err := s.Consume(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.IV, p.IV) || !bytes.Equal(got.Ciphertext, p.Ciphertext) {
		t.Fatalf("payload mismatch: %q", got)
	}
	if _, err := s.Consume(ctx, id); !errors.Is(err, ErrConsumed) {
		t.Fatalf("second consume = %v", err)
	}
	st, exp, err = s.Status(ctx, id)
	if err != nil || st != Consumed || exp.IsZero() {
		t.Fatalf("post-consume status = %v %v %v", st, exp, err)
	}
}

func TestStatusNonDestructive(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	id := newID(t, s)
	if st, exp, err := s.Status(ctx, id); err != nil || st != Gone || !exp.IsZero() {
		t.Fatalf("unknown: %v %v %v", st, exp, err)
	}
	if err := s.Create(ctx, id, payload(), time.Minute); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if st, _, _ := s.Status(ctx, id); st != Available {
			t.Fatalf("status burned secret: %v", st)
		}
	}
	if _, err := s.Consume(ctx, id); err != nil {
		t.Fatal(err)
	}
}

func TestConsumeNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Consume(context.Background(), newID(t, s)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestCreateCollision(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	id := newID(t, s)
	if err := s.Create(ctx, id, payload(), time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := s.Create(ctx, id, payload(), time.Hour); !errors.Is(err, ErrIDCollision) {
		t.Fatalf("secret collision = %v", err)
	}
	if _, err := s.Consume(ctx, id); err != nil {
		t.Fatal(err)
	}
	// tombstone also collides
	if err := s.Create(ctx, id, payload(), time.Hour); !errors.Is(err, ErrIDCollision) {
		t.Fatalf("tombstone collision = %v", err)
	}
}

func TestCreateValidation(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	id := newID(t, s)
	if err := s.Create(ctx, id, Payload{IV: []byte("short"), Ciphertext: []byte("x")}, time.Hour); err == nil {
		t.Fatal("expected bad iv error")
	}
	if err := s.Create(ctx, id, payload(), 0); err == nil {
		t.Fatal("expected bad ttl error")
	}
}

func TestConcurrentConsume(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	id := newID(t, s)
	if err := s.Create(ctx, id, payload(), time.Minute); err != nil {
		t.Fatal(err)
	}
	const n = 200
	var ok, consumed, other atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := s.Consume(ctx, id)
			switch {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, ErrConsumed):
				consumed.Add(1)
			default:
				other.Add(1)
				t.Errorf("unexpected: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()
	if ok.Load() != 1 || consumed.Load() != n-1 || other.Load() != 0 {
		t.Fatalf("ok=%d consumed=%d other=%d", ok.Load(), consumed.Load(), other.Load())
	}
}

func TestTombstoneTTL(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	id := newID(t, s)
	if err := s.Create(ctx, id, payload(), 1500*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	if _, err := s.Consume(ctx, id); err != nil {
		t.Fatal(err)
	}
	// Tombstone TTL must be the remaining original TTL (<= ~1000ms), not the full one.
	d, err := s.rdb.PTTL(ctx, tombPrefix+id).Result()
	if err != nil || d <= 0 || d > 1100*time.Millisecond {
		t.Fatalf("tombstone pttl = %v, %v", d, err)
	}
	if exists, _ := s.rdb.Exists(ctx, secretPrefix+id).Result(); exists != 0 {
		t.Fatal("secret key still present")
	}
	if _, err := s.Consume(ctx, id); !errors.Is(err, ErrConsumed) {
		t.Fatalf("got %v", err)
	}
	time.Sleep(1300 * time.Millisecond)
	if _, err := s.Consume(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after expiry got %v", err)
	}
	if st, _, _ := s.Status(ctx, id); st != Gone {
		t.Fatalf("status after expiry = %v", st)
	}
}

func TestSecretExpires(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	id := newID(t, s)
	if err := s.Create(ctx, id, payload(), 200*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(400 * time.Millisecond)
	if _, err := s.Consume(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestPing(t *testing.T) {
	s := newTestStore(t)
	if err := s.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestPingDown(t *testing.T) {
	c := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1, DialTimeout: 200 * time.Millisecond})
	defer c.Close()
	r := &Redis{rdb: c, now: time.Now}
	if err := r.Ping(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}
