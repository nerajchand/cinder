package integration

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

// TestCLIRoundTrip drives scripts/cinder-cli.mjs against the full handler stack:
// create, fetch (plaintext matches), second fetch fails with already_viewed.
func TestCLIRoundTrip(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	e := memEnv(t)
	script := "../../scripts/cinder-cli.mjs"
	run := func(stdin string, args ...string) (string, string, error) {
		cmd := exec.Command(node, append([]string{script}, args...)...)
		cmd.Stdin = strings.NewReader(stdin)
		var out, errb bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errb
		err := cmd.Run()
		return out.String(), errb.String(), err
	}

	const secret = "pässword ✓ \"q\"\nline2"
	link, stderr, err := run(secret, "create", "--url", e.srv.URL, "--ttl", "300")
	if err != nil {
		t.Fatalf("create: %v %s", err, stderr)
	}
	link = strings.TrimSpace(link)
	if !strings.HasPrefix(link, e.srv.URL+"/s/") || !strings.Contains(link, "#") {
		t.Fatalf("unexpected link %q", link)
	}
	got, stderr, err := run("", "fetch", link)
	if err != nil || got != secret {
		t.Fatalf("fetch: %q %v %s", got, err, stderr)
	}
	_, stderr, err = run("", "fetch", link)
	if err == nil || !strings.Contains(stderr, "already_viewed") {
		t.Fatalf("second fetch should fail with already_viewed: %v %s", err, stderr)
	}
}
