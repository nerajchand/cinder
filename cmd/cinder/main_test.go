package main

import (
	"testing"
)

func TestUseMemory(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	cases := []struct {
		env  map[string]string
		want bool
	}{
		{map[string]string{}, true},
		{map[string]string{"CINDER_REDIS_URL": "redis://x:6379/0"}, false},
		{map[string]string{"CINDER_REDIS_URL": "redis://x:6379/0", "CINDER_STORE": "memory"}, true},
	}
	for i, c := range cases {
		if got := useMemory(env(c.env)); got != c.want {
			t.Errorf("case %d: got %v", i, got)
		}
	}
}
