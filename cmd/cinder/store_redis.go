package main

import (
	"errors"

	"github.com/cinder/cinder/internal/store"
)

// TODO(harden): wire the Redis-backed store here. The harden task replaces
// this stub with a real implementation of store.Store using redisURL.
func newRedisStore(redisURL string) (store.Store, error) {
	return nil, errors.New("redis store: not wired")
}
