// Command cinder is the Cinder burn-on-read secret sharing server.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/cinder/cinder/internal/app"
	"github.com/cinder/cinder/internal/config"
	"github.com/cinder/cinder/internal/store"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		if err := healthcheck(os.Getenv, http.DefaultClient); err != nil {
			fmt.Fprintln(os.Stderr, "cinder healthcheck:", err)
			os.Exit(1)
		}
		return
	}
	if err := run(os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, "cinder:", err)
		os.Exit(1)
	}
}

// useMemory reports whether the in-memory store should be used.
func useMemory(getenv func(string) string) bool {
	return strings.TrimSpace(getenv("CINDER_REDIS_URL")) == "" ||
		strings.EqualFold(strings.TrimSpace(getenv("CINDER_STORE")), "memory")
}

func run(getenv func(string) string) error {
	cfg, err := config.Load(getenv)
	if err != nil {
		return fmt.Errorf("config error: %w", err)
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var st store.Store
	if useMemory(getenv) {
		mem := store.NewMemory()
		go mem.RunJanitor(ctx, time.Minute)
		st = mem
		log.Warn("using in-memory store; secrets are lost on restart")
	} else {
		var closeStore func() error
		if st, closeStore, err = newRedisStore(ctx, cfg.RedisURL); err != nil {
			return err
		}
		defer closeStore()
	}

	handler := app.NewHandler(cfg, st, log)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}

	errc := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.Addr)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	sctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		return err
	}
	if err := <-errc; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
