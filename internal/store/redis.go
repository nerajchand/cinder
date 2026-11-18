package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	ivLen         = 12
	secretPrefix  = "cinder:s:"
	tombPrefix    = "cinder:t:"
	minRedisMajor = 6
	minRedisMinor = 2
)

// Redis is a Redis-backed Store.
type Redis struct {
	rdb redis.UniversalClient
	now func() time.Time
}

var _ Store = (*Redis)(nil)

// NewRedis connects using a redis:// or rediss:// URL and verifies the server
// version is >= 6.2.
func NewRedis(ctx context.Context, url string) (*Redis, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("store: parse redis url: %w", err)
	}
	r, err := NewRedisFromClient(ctx, redis.NewClient(opts))
	if err != nil {
		return nil, err
	}
	return r, nil
}

// NewRedisFromClient wraps an existing client and verifies the server version.
// On error the client is closed.
func NewRedisFromClient(ctx context.Context, c redis.UniversalClient) (*Redis, error) {
	r := &Redis{rdb: c, now: time.Now}
	if err := r.checkVersion(ctx); err != nil {
		_ = c.Close()
		return nil, err
	}
	return r, nil
}

// Close releases the underlying client.
func (r *Redis) Close() error { return r.rdb.Close() }

func (r *Redis) checkVersion(ctx context.Context) error {
	info, err := r.rdb.Info(ctx, "server").Result()
	if err != nil {
		return fmt.Errorf("store: redis INFO: %w", err)
	}
	for _, line := range strings.Split(info, "\n") {
		v, ok := strings.CutPrefix(strings.TrimSpace(line), "redis_version:")
		if !ok {
			continue
		}
		parts := strings.Split(v, ".")
		if len(parts) < 2 {
			break
		}
		maj, e1 := strconv.Atoi(parts[0])
		min, e2 := strconv.Atoi(parts[1])
		if e1 != nil || e2 != nil {
			break
		}
		if maj < minRedisMajor || (maj == minRedisMajor && min < minRedisMinor) {
			return fmt.Errorf("store: redis %s too old, need >= %d.%d", v, minRedisMajor, minRedisMinor)
		}
		return nil
	}
	return errors.New("store: cannot determine redis version")
}

func keys(id string) []string { return []string{secretPrefix + id, tombPrefix + id} }

// Create implements Store.
func (r *Redis) Create(ctx context.Context, id string, p Payload, ttl time.Duration) error {
	if len(p.IV) != ivLen {
		return fmt.Errorf("store: iv must be %d bytes", ivLen)
	}
	ms := ttl.Milliseconds()
	if ms <= 0 {
		return errors.New("store: ttl must be at least 1ms")
	}
	val := make([]byte, 0, ivLen+len(p.Ciphertext))
	val = append(append(val, p.IV...), p.Ciphertext...)
	n, err := createScript.Run(ctx, r.rdb, keys(id), val, ms).Int()
	if err != nil {
		return fmt.Errorf("store: create: %w", err)
	}
	if n == 0 {
		return ErrIDCollision
	}
	return nil
}

// Status implements Store.
func (r *Redis) Status(ctx context.Context, id string) (State, time.Time, error) {
	res, err := statusScript.Run(ctx, r.rdb, keys(id)).Int64Slice()
	if err != nil {
		return Gone, time.Time{}, fmt.Errorf("store: status: %w", err)
	}
	if len(res) != 2 {
		return Gone, time.Time{}, errors.New("store: status: bad reply")
	}
	exp := r.now().Add(time.Duration(res[1]) * time.Millisecond).UTC()
	switch res[0] {
	case 1:
		return Available, exp, nil
	case 2:
		return Consumed, exp, nil
	}
	return Gone, time.Time{}, nil
}

// Consume implements Store.
func (r *Redis) Consume(ctx context.Context, id string) (Payload, error) {
	res, err := consumeScript.Run(ctx, r.rdb, keys(id)).Slice()
	if err != nil {
		return Payload{}, fmt.Errorf("store: consume: %w", err)
	}
	if len(res) == 0 {
		return Payload{}, errors.New("store: consume: empty reply")
	}
	tag, _ := res[0].(string)
	switch tag {
	case "consumed":
		return Payload{}, ErrConsumed
	case "notfound":
		return Payload{}, ErrNotFound
	case "ok":
		if len(res) < 2 {
			break
		}
		v, ok := res[1].(string)
		if !ok || len(v) <= ivLen {
			return Payload{}, errors.New("store: consume: corrupt value")
		}
		b := []byte(v)
		return Payload{IV: b[:ivLen:ivLen], Ciphertext: b[ivLen:]}, nil
	}
	return Payload{}, errors.New("store: consume: unexpected reply")
}

// Ping implements Store.
func (r *Redis) Ping(ctx context.Context) error {
	if err := r.rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("store: ping: %w", err)
	}
	return nil
}
