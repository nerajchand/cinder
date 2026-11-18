// Command cinder is the Cinder burn-on-read secret sharing server.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/cinder/cinder/internal/api"
	"github.com/cinder/cinder/internal/config"
	"github.com/cinder/cinder/internal/store"
)

func main() {
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
	} else if st, err = newRedisStore(cfg.RedisURL); err != nil {
		return err
	}

	// TODO(web): pass the static handler from internal/web once available.
	// TODO(middleware): pass the middleware chain from internal/middleware.
	handler := api.NewRouter(api.Deps{
		Store:              st,
		MaxCiphertextBytes: cfg.MaxCiphertextBytes,
		AllowedOrigins:     cfg.AllowedOrigins,
		RequestScheme:      schemeFunc(cfg.TrustedProxies),
		Logger:             log,
	}, nil)

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

// schemeFunc returns the client-visible scheme: X-Forwarded-Proto is only
// honoured when the direct peer is a trusted proxy.
func schemeFunc(trusted []netip.Prefix) func(*http.Request) string {
	return func(r *http.Request) string {
		if r.TLS != nil {
			return "https"
		}
		if len(trusted) > 0 {
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err == nil {
				if a, err := netip.ParseAddr(host); err == nil {
					a = a.Unmap()
					for _, p := range trusted {
						if p.Contains(a) {
							if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
								return "https"
							}
							break
						}
					}
				}
			}
		}
		return "http"
	}
}
