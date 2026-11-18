package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/cinder/cinder/internal/store"
)

type downStore struct{ store.Store }

func (downStore) Ping(context.Context) error { return errors.New("down") }

func TestHealthzReflectsStore(t *testing.T) {
	if code, _, _ := memEnv(t).do(t, "GET", "/healthz", "", nil); code != 200 {
		t.Fatalf("healthy: %d", code)
	}
	e := newEnv(t, downStore{store.NewMemory()})
	code, _, b := e.do(t, "GET", "/healthz", "", nil)
	if code != 503 {
		t.Fatalf("down: %d %s", code, b)
	}
}
