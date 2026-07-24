package coverage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// Runner generates coverage profiles by invoking `go test -coverprofile`,
// mirroring crap4java's CoverageRunner/ProcessCommandExecutor.
//
// Generate may be called concurrently for different module roots. Stdout and
// Stderr must be safe for concurrent Write calls; NewRunner wraps the process
// streams with SyncWriter for that purpose.
type Runner struct {
	Stdout io.Writer
	Stderr io.Writer
}

// NewRunner returns a Runner that streams the `go test` output to the
// process's stdout/stderr, mirroring ProcessBuilder.inheritIO(). Writers are
// synchronized so parallel Generate calls do not race.
func NewRunner() *Runner {
	return &Runner{Stdout: SyncWriter(os.Stdout), Stderr: SyncWriter(os.Stderr)}
}

// Generate runs `go test ./... -coverprofile=<tmp>` in moduleRoot.
//
// ctx bounds the command: if ctx is canceled (e.g. Ctrl-C) or its deadline
// expires, the `go test` process is killed and Generate returns ctx.Err().
//
// A non-zero exit caused by failing tests is reported via testsFailed=true
// but is not treated as an error: whatever coverage was recorded up to the
// failures is still useful. err is only set when the command itself could
// not be run at all (e.g. the go toolchain is missing), matching
// crap4java's "coverage command failure" fail-fast behavior.
//
// Generate may be invoked concurrently for distinct moduleRoot values when
// Stdout/Stderr are concurrency-safe (as with NewRunner / SyncWriter).
func (r *Runner) Generate(ctx context.Context, moduleRoot string) (string, bool, error) {
	tmp, err := os.CreateTemp("", "go-check-it-coverage-*.out")
	if err != nil {
		return "", false, err
	}
	profilePath := tmp.Name()
	if err := tmp.Close(); err != nil {
		return "", false, err
	}

	cmd := exec.CommandContext(ctx, "go", "test", "./...", "-coverprofile="+profilePath)
	cmd.Dir = moduleRoot
	cmd.Stdout = r.Stdout
	cmd.Stderr = r.Stderr

	runErr := cmd.Run()
	if ctx.Err() != nil {
		return profilePath, false, ctx.Err()
	}
	if runErr == nil {
		return profilePath, false, nil
	}

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return profilePath, true, nil
	}
	return profilePath, false, fmt.Errorf("coverage command failed to run: %w", runErr)
}
