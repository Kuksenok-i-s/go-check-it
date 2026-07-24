// Command go-check-it computes CRAP (Change Risk Anti-Patterns) scores for Go
// functions by combining cyclomatic complexity with `go test` coverage.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"go-check-it/internal/cliapp"
)

func main() {
	os.Exit(run())
}

func run() int {
	projectRoot, err := filepath.Abs(".")
	if err != nil {
		fmt.Fprintln(os.Stderr, "go-check-it:", err)
		return 1
	}

	// Canceling ctx on SIGINT/SIGTERM lets a running `go test` subprocess be
	// killed promptly instead of leaving Ctrl-C to hang until it finishes.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app := cliapp.New(projectRoot, os.Stdout, os.Stderr)
	return app.Execute(ctx, os.Args[1:])
}
