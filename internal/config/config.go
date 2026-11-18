// Package config parses and validates CINDER_* environment variables.
package config

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

const (
	DefaultAddr               = ":8080"
	DefaultRedisURL           = "redis://localhost:6379/0"
	DefaultMaxCiphertextBytes = 64 * 1024
	// HardMaxCiphertextBytes is the absolute upper bound for the limit.
	HardMaxCiphertextBytes = 1 << 20
)

// Config is the validated runtime configuration.
type Config struct {
	Addr                   string
	RedisURL               string
	MaxCiphertextBytes     int
	TrustedProxies         []netip.Prefix
	AllowedOrigins         []string // normalized scheme://host[:port]
	RateLimitCreatePerMin  int
	RateLimitConsumePerMin int
	RateLimitStatusPerMin  int
	RateLimitBurst         int
}

// Load reads configuration using getenv (typically os.Getenv).
func Load(getenv func(string) string) (Config, error) {
	c := Config{
		Addr:                   str(getenv, "CINDER_ADDR", DefaultAddr),
		RedisURL:               str(getenv, "CINDER_REDIS_URL", DefaultRedisURL),
		MaxCiphertextBytes:     DefaultMaxCiphertextBytes,
		RateLimitCreatePerMin:  30,
		RateLimitConsumePerMin: 60,
		RateLimitStatusPerMin:  120,
		RateLimitBurst:         10,
	}
	var errs []error
	add := func(err error) {
		if err != nil {
			errs = append(errs, err)
		}
	}

	if !strings.Contains(c.Addr, ":") {
		add(fmt.Errorf("CINDER_ADDR %q must be host:port or :port", c.Addr))
	}
	if u, err := url.Parse(c.RedisURL); err != nil || (u.Scheme != "redis" && u.Scheme != "rediss") || u.Host == "" {
		add(fmt.Errorf("CINDER_REDIS_URL must be a redis:// or rediss:// URL"))
	}

	if v := getenv("CINDER_MAX_CIPHERTEXT_BYTES"); v != "" {
		n, err := strconv.Atoi(v)
		switch {
		case err != nil:
			add(fmt.Errorf("CINDER_MAX_CIPHERTEXT_BYTES: %w", err))
		case n < 1 || n > HardMaxCiphertextBytes:
			add(fmt.Errorf("CINDER_MAX_CIPHERTEXT_BYTES must be between 1 and %d", HardMaxCiphertextBytes))
		default:
			c.MaxCiphertextBytes = n
		}
	}

	for _, item := range split(getenv("CINDER_TRUSTED_PROXIES")) {
		if p, err := netip.ParsePrefix(item); err == nil {
			c.TrustedProxies = append(c.TrustedProxies, p.Masked())
		} else if a, err := netip.ParseAddr(item); err == nil {
			c.TrustedProxies = append(c.TrustedProxies, netip.PrefixFrom(a, a.BitLen()))
		} else {
			add(fmt.Errorf("CINDER_TRUSTED_PROXIES: invalid IP or CIDR %q", item))
		}
	}

	for _, item := range split(getenv("CINDER_ALLOWED_ORIGINS")) {
		u, err := url.Parse(item)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
			(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
			add(fmt.Errorf("CINDER_ALLOWED_ORIGINS: invalid origin %q (want scheme://host[:port])", item))
			continue
		}
		c.AllowedOrigins = append(c.AllowedOrigins, strings.ToLower(u.Scheme+"://"+u.Host))
	}

	for _, r := range []struct {
		name string
		dst  *int
		min  int
	}{
		{"CINDER_RATE_LIMIT_CREATE_PER_MIN", &c.RateLimitCreatePerMin, 0},
		{"CINDER_RATE_LIMIT_CONSUME_PER_MIN", &c.RateLimitConsumePerMin, 0},
		{"CINDER_RATE_LIMIT_STATUS_PER_MIN", &c.RateLimitStatusPerMin, 0},
		{"CINDER_RATE_LIMIT_BURST", &c.RateLimitBurst, 1},
	} {
		if v := getenv(r.name); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < r.min || n > 1_000_000 {
				add(fmt.Errorf("%s must be an integer between %d and 1000000 (0 disables, where allowed)", r.name, r.min))
				continue
			}
			*r.dst = n
		}
	}

	if len(errs) > 0 {
		return Config{}, errors.Join(errs...)
	}
	return c, nil
}

func str(getenv func(string) string, key, def string) string {
	if v := strings.TrimSpace(getenv(key)); v != "" {
		return v
	}
	return def
}

func split(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
