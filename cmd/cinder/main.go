// Command cinder is the Cinder burn-on-read secret sharing server.
package main

import (
	"fmt"
	"os"

	"github.com/cinder/cinder/internal/config"
)

func main() {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		os.Exit(2)
	}
	// Placeholder: server wiring is added by later tasks.
	fmt.Fprintln(os.Stderr, "cinder: scaffold only; would listen on", cfg.Addr)
}
