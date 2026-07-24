package coverage

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeModule(t *testing.T, root string) {
	t.Helper()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/covtest\n\ngo 1.23\n"), 0o644))
	must(os.WriteFile(filepath.Join(root, "add.go"), []byte(`package covtest

func Add(a, b int) int {
	if a < 0 {
		return b
	}
	return a + b
}
`), 0o644))
	must(os.WriteFile(filepath.Join(root, "add_test.go"), []byte(`package covtest

import "testing"

func TestAdd(t *testing.T) {
	if Add(1, 2) != 3 {
		t.Fatal("bad")
	}
}
`), 0o644))
}

func TestRunner_Generate_ProducesProfile(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root)

	var stdout, stderr bytes.Buffer
	r := &Runner{Stdout: &stdout, Stderr: &stderr}
	profilePath, testsFailed, err := r.Generate(context.Background(), root)
	defer os.Remove(profilePath)
	if err != nil {
		t.Fatalf("unexpected error: %v (stderr: %s)", err, stderr.String())
	}
	if testsFailed {
		t.Fatalf("expected tests to pass, stderr: %s", stderr.String())
	}
	info, statErr := os.Stat(profilePath)
	if statErr != nil {
		t.Fatalf("expected coverage profile to exist: %v", statErr)
	}
	if info.Size() == 0 {
		t.Fatal("expected non-empty coverage profile")
	}

	index, err := ParseProfile(profilePath, root)
	if err != nil {
		t.Fatal(err)
	}
	total, covered := FunctionCoverage(index, filepath.Join(root, "add.go"), 3, 8)
	if total == 0 {
		t.Fatal("expected coverage blocks for Add")
	}
	if covered == 0 {
		t.Fatal("expected some covered statements for Add")
	}
}

func TestRunner_Generate_ContextCanceledReturnsContextError(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root)

	var stdout, stderr bytes.Buffer
	r := &Runner{Stdout: &stdout, Stderr: &stderr}

	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	<-ctx.Done()

	profilePath, testsFailed, err := r.Generate(ctx, root)
	defer os.Remove(profilePath)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
	if testsFailed {
		t.Fatalf("expected testsFailed=false on cancellation, got true")
	}
}
