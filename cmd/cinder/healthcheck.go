package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/cinder/cinder/internal/config"
)

// healthURL derives the local /healthz URL from CINDER_ADDR.
func healthURL(getenv func(string) string) (string, error) {
	addr := config.DefaultAddr
	if v := strings.TrimSpace(getenv("CINDER_ADDR")); v != "" {
		addr = v
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("CINDER_ADDR: %w", err)
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/healthz", nil
}

// healthcheck GETs /healthz on the local listener; used by the container
// HEALTHCHECK because the distroless image has no shell, curl or wget.
func healthcheck(getenv func(string) string, client *http.Client) error {
	u, err := healthURL(getenv)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: status %d", u, resp.StatusCode)
	}
	return nil
}
