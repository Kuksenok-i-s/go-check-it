package a

import (
	"context"
	"net/http"
	"os/exec"
)

func bad() {
	exec.Command("ls") // want `exec.Command ignores context cancellation; use exec.CommandContext instead`
}

func badHTTP() {
	http.Get("http://example.com") // want `http.Get ignores context cancellation`
}

func good(ctx context.Context) {
	exec.CommandContext(ctx, "ls")
}
