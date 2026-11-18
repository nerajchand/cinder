package main

import (
	"context"
	"fmt"
	"time"

	"github.com/cinder/cinder/internal/store"
)

// newRedisStore connects to Redis (verifying the server version) and returns
// the store plus a close function.
func newRedisStore(ctx context.Context, redisURL string) (store.Store, func() error, error) {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	r, err := store.NewRedis(cctx, redisURL)
	if err != nil {
		return nil, nil, fmt.Errorf("redis store: %w", err)
	}
	return r, r.Close, nil
}
