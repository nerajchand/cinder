package config

import (
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestDefaults(t *testing.T) {
	c, err := Load(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.Addr != ":8080" || c.RedisURL != "redis://localhost:6379/0" || c.MaxCiphertextBytes != 65536 {
		t.Fatalf("bad defaults: %+v", c)
	}
	if len(c.TrustedProxies) != 0 || len(c.AllowedOrigins) != 0 {
		t.Fatalf("expected empty lists: %+v", c)
	}
}

func TestOverrides(t *testing.T) {
	c, err := Load(env(map[string]string{
		"CINDER_ADDR":                      "127.0.0.1:9000",
		"CINDER_REDIS_URL":                 "rediss://u:p@redis:6380/1",
		"CINDER_MAX_CIPHERTEXT_BYTES":      "1048576",
		"CINDER_TRUSTED_PROXIES":           "10.0.0.0/8, 192.168.1.5",
		"CINDER_ALLOWED_ORIGINS":           "https://Cinder.Example.com, http://localhost:8080/",
		"CINDER_RATE_LIMIT_BURST":          "5",
		"CINDER_RATE_LIMIT_CREATE_PER_MIN": "0",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.MaxCiphertextBytes != 1<<20 || len(c.TrustedProxies) != 2 || c.TrustedProxies[1].String() != "192.168.1.5/32" {
		t.Fatalf("bad: %+v", c)
	}
	if c.AllowedOrigins[0] != "https://cinder.example.com" || c.AllowedOrigins[1] != "http://localhost:8080" {
		t.Fatalf("bad origins: %v", c.AllowedOrigins)
	}
	if c.RateLimitBurst != 5 || c.RateLimitCreatePerMin != 0 {
		t.Fatalf("bad rate: %+v", c)
	}
}

func TestInvalid(t *testing.T) {
	cases := map[string]map[string]string{
		"addr":        {"CINDER_ADDR": "nonsense"},
		"redis":       {"CINDER_REDIS_URL": "http://x"},
		"max zero":    {"CINDER_MAX_CIPHERTEXT_BYTES": "0"},
		"max over":    {"CINDER_MAX_CIPHERTEXT_BYTES": "1048577"},
		"max nan":     {"CINDER_MAX_CIPHERTEXT_BYTES": "abc"},
		"proxy":       {"CINDER_TRUSTED_PROXIES": "not-an-ip"},
		"origin":      {"CINDER_ALLOWED_ORIGINS": "example.com"},
		"origin path": {"CINDER_ALLOWED_ORIGINS": "https://example.com/x"},
		"rate neg":    {"CINDER_RATE_LIMIT_STATUS_PER_MIN": "-1"},
		"burst zero":  {"CINDER_RATE_LIMIT_BURST": "0"},
	}
	for name, m := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(env(m)); err == nil {
				t.Fatal("expected error")
			} else if !strings.Contains(err.Error(), "CINDER_") {
				t.Fatalf("error should name variable: %v", err)
			}
		})
	}
}
