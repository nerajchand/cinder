package idgen

import (
	"strings"
	"testing"
)

func TestNewValidAndUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id, err := New()
		if err != nil {
			t.Fatal(err)
		}
		if len(id) != Length || !Valid(id) {
			t.Fatalf("bad id %q", id)
		}
		if strings.ContainsAny(id, "=+/") {
			t.Fatalf("not base64url unpadded: %q", id)
		}
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
}

func TestValid(t *testing.T) {
	cases := map[string]bool{
		"":                        false,
		"AAAAAAAAAAAAAAAAAAAAAA":  true,
		"AAAAAAAAAAAAAAAAAAAAAB":  false, // non-canonical trailing bits
		"AAAAAAAAAAAAAAAAAAAAA":   false, // short
		"AAAAAAAAAAAAAAAAAAAAAAA": false, // long
		"AAAAAAAAAAAAAAAAAAAAA=":  false,
		"AAAAAAAAAAAAAAAAAAAA+A":  false,
		"AAAAAAAAAAAAAAAAAAAA/A":  false,
		"AAAAAAAAAAAAAAAAAAAA A":  false,
		"../../etc/passwd/xxxxx":  false,
	}
	for in, want := range cases {
		if got := Valid(in); got != want {
			t.Errorf("Valid(%q)=%v want %v", in, got, want)
		}
	}
}
