// Package app assembles the full Cinder HTTP handler stack. It exists so the
// binary and the integration tests exercise exactly the same wiring.
package app

import (
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"

	"github.com/cinder/cinder/internal/api"
	"github.com/cinder/cinder/internal/config"
	"github.com/cinder/cinder/internal/middleware"
	"github.com/cinder/cinder/internal/store"
	"github.com/cinder/cinder/internal/web"
)

// NewHandler returns the router with the embedded web UI and the standard
// middleware chain.
//
// The consume Origin check is performed exactly once, inside the api consume
// handler, using middleware.OriginChecker (rejects missing/"null"/multiple
// Origin headers; own origin honours trusted-proxy scheme). middleware
// RequireOrigin is deliberately NOT layered on top, so there is a single
// authoritative check.
func NewHandler(cfg config.Config, st store.Store, log *slog.Logger) http.Handler {
	return api.NewRouter(api.Deps{
		Store:              st,
		MaxCiphertextBytes: cfg.MaxCiphertextBytes,
		AllowedOrigins:     cfg.AllowedOrigins,
		OriginCheck:        middleware.OriginChecker(cfg.AllowedOrigins, cfg.TrustedProxies),
		RequestScheme:      SchemeFunc(cfg.TrustedProxies),
		Logger:             log,
	}, web.Handler(), middleware.New(cfg, middleware.WithLogger(log)))
}

// SchemeFunc returns the client-visible scheme: X-Forwarded-Proto is only
// honoured when the direct peer is a trusted proxy.
func SchemeFunc(trusted []netip.Prefix) func(*http.Request) string {
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
