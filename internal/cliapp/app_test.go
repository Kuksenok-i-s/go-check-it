package cliapp

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeCoverage implements CoverageGenerator without invoking `go test`.
type fakeCoverage struct {
	mu sync.Mutex

	profilePath string
	testsFailed bool
	err         error

	// perModule overrides Generate results by module root basename or path.
	perModule map[string]fakeModuleResult

	delay time.Duration

	// blockUntil, when non-nil, blocks Generate until that context is done
	// (or the call's ctx is canceled).
	blockUntil context.Context

	calls       []string
	maxInFlight int32
	inFlight    int32
}

type fakeModuleResult struct {
	profilePath string
	testsFailed bool
	err         error
}

func (f *fakeCoverage) Generate(ctx context.Context, moduleRoot string) (string, bool, error) {
	cur := atomic.AddInt32(&f.inFlight, 1)
	for {
		old := atomic.LoadInt32(&f.maxInFlight)
		if cur <= old || atomic.CompareAndSwapInt32(&f.maxInFlight, old, cur) {
			break
		}
	}
	defer atomic.AddInt32(&f.inFlight, -1)

	f.mu.Lock()
	f.calls = append(f.calls, moduleRoot)
	f.mu.Unlock()

	if f.blockUntil != nil {
		select {
		case <-f.blockUntil.Done():
		case <-ctx.Done():
			return "", false, ctx.Err()
		}
	}
	if f.delay > 0 {
		timer := time.NewTimer(f.delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return "", false, ctx.Err()
		}
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if f.perModule != nil {
		if r, ok := f.perModule[moduleRoot]; ok {
			return r.profilePath, r.testsFailed, r.err
		}
		if r, ok := f.perModule[filepath.Base(moduleRoot)]; ok {
			return r.profilePath, r.testsFailed, r.err
		}
	}
	return f.profilePath, f.testsFailed, f.err
}

func (f *fakeCoverage) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func writeGoFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newTestApp(t *testing.T, root string, gen CoverageGenerator) (*App, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	app := &App{ProjectRoot: root, Stdout: &stdout, Stderr: &stderr, Coverage: gen}
	return app, &stdout, &stderr
}

func TestExecute_HelpPrintsUsageAndExitsZero(t *testing.T) {
	app, stdout, _ := newTestApp(t, t.TempDir(), &fakeCoverage{})
	code := app.Execute(context.Background(), []string{"--help"})
	if code != ExitOK {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "Usage:") {
		t.Fatalf("expected usage text, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "--max-workers") {
		t.Fatalf("expected --max-workers in usage, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "--threshold") {
		t.Fatalf("expected --threshold in usage, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "agent-json") {
		t.Fatalf("expected agent-json in usage, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "--top") {
		t.Fatalf("expected --top in usage, got %q", stdout.String())
	}
}

func TestExecute_UsageErrorExitsOne(t *testing.T) {
	app, _, stderr := newTestApp(t, t.TempDir(), &fakeCoverage{})
	code := app.Execute(context.Background(), []string{"--changed", "file.go"})
	if code != ExitUsageError {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "--changed cannot be combined") {
		t.Fatalf("expected usage error message, got %q", stderr.String())
	}
}

func TestExecute_EmptySelectionExitsZero(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module empty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	app, stdout, _ := newTestApp(t, root, &fakeCoverage{})
	code := app.Execute(context.Background(), nil)
	if code != ExitOK {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "No Go files to analyze.") {
		t.Fatalf("expected empty-selection message, got %q", stdout.String())
	}
}

func TestExecute_MissingCoverageProfileReportsNA(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, filepath.Join(root, "go.mod"), "module m\n")
	writeGoFile(t, filepath.Join(root, "risky.go"), `package m

func Risky(a int) int {
	if a > 0 {
		return a
	}
	return -a
}
`)

	gen := &fakeCoverage{profilePath: filepath.Join(root, "missing.out")}
	app, stdout, stderr := newTestApp(t, root, gen)
	code := app.Execute(context.Background(), nil)
	if code != ExitOK {
		t.Fatalf("expected exit 0 (complexity 2 is below threshold), got %d", code)
	}
	if !strings.Contains(stdout.String(), "Risky") || !strings.Contains(stdout.String(), "N/A") {
		t.Fatalf("expected Risky row with N/A coverage, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Warning: coverage profile not found") {
		t.Fatalf("expected coverage warning, got %q", stderr.String())
	}
}

func TestExecute_ThresholdExceededExitsTwo(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, filepath.Join(root, "go.mod"), "module m\n")
	writeGoFile(t, filepath.Join(root, "risky.go"), `package m

func Risky(a, b, c int) int {
	if a > 0 && b > 0 {
		if c > 0 {
			return a + b + c
		}
		return a
	}
	for i := 0; i < a; i++ {
		b += i
	}
	switch {
	case b > 10:
		return b
	case b < 0:
		return -b
	default:
		return 0
	}
}
`)

	profile := filepath.Join(root, "cover.out")
	writeGoFile(t, profile, "mode: set\nm/risky.go:4.30,18.2 8 0\n")

	gen := &fakeCoverage{profilePath: profile}
	app, stdout, stderr := newTestApp(t, root, gen)
	code := app.Execute(context.Background(), nil)
	if code != ExitThresholdExceeded {
		t.Fatalf("expected exit 2, got %d (stdout: %s)", code, stdout.String())
	}
	if !strings.Contains(stderr.String(), "CRAP threshold exceeded") {
		t.Fatalf("expected threshold message, got %q", stderr.String())
	}
}

func TestExecute_AgentJSONHotspotsAndExitTwo(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, filepath.Join(root, "go.mod"), "module m\n")
	writeGoFile(t, filepath.Join(root, "risky.go"), `package m

func Risky(a, b, c int) int {
	if a > 0 && b > 0 {
		if c > 0 {
			return a + b + c
		}
		return a
	}
	for i := 0; i < a; i++ {
		b += i
	}
	switch {
	case b > 10:
		return b
	case b < 0:
		return -b
	default:
		return 0
	}
}
`)
	writeGoFile(t, filepath.Join(root, "safe.go"), `package m

func Safe() int { return 1 }
`)

	profile := filepath.Join(root, "cover.out")
	writeGoFile(t, profile, "mode: set\nm/risky.go:4.30,18.2 8 0\nm/safe.go:3.18,3.27 1 1\n")

	gen := &fakeCoverage{profilePath: profile}
	app, stdout, stderr := newTestApp(t, root, gen)
	code := app.Execute(context.Background(), []string{"--format=agent-json", "--top=1", "--fail-on-findings"})
	if code != ExitThresholdExceeded {
		t.Fatalf("expected exit 2, got %d (stdout: %s stderr: %s)", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, `"hotspots"`) || !strings.Contains(out, `"omitted"`) {
		t.Fatalf("expected agent-json keys, got %q", out)
	}
	if !strings.Contains(out, "Risky") {
		t.Fatalf("expected Risky hotspot, got %q", out)
	}
	if strings.Contains(out, `"functions"`) {
		t.Fatalf("agent-json must not include full functions list: %q", out)
	}
}

func TestExecute_CustomThresholdRaisesBar(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, filepath.Join(root, "go.mod"), "module m\n")
	writeGoFile(t, filepath.Join(root, "risky.go"), `package m

func Risky(a, b, c int) int {
	if a > 0 && b > 0 {
		if c > 0 {
			return a + b + c
		}
		return a
	}
	for i := 0; i < a; i++ {
		b += i
	}
	switch {
	case b > 10:
		return b
	case b < 0:
		return -b
	default:
		return 0
	}
}
`)

	profile := filepath.Join(root, "cover.out")
	writeGoFile(t, profile, "mode: set\nm/risky.go:4.30,18.2 8 0\n")

	gen := &fakeCoverage{profilePath: profile}
	app, stdout, _ := newTestApp(t, root, gen)
	code := app.Execute(context.Background(), []string{"--threshold=100"})
	if code != ExitOK {
		t.Fatalf("expected exit 0 with raised threshold, got %d (stdout: %s)", code, stdout.String())
	}
}

func TestExecute_InvalidThresholdExitsOne(t *testing.T) {
	app, _, stderr := newTestApp(t, t.TempDir(), &fakeCoverage{})
	code := app.Execute(context.Background(), []string{"--threshold=abc"})
	if code != ExitUsageError {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "--threshold") {
		t.Fatalf("expected threshold error message, got %q", stderr.String())
	}
}

func TestExecute_ExplicitDirectoryArgExpandsToGoFiles(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, filepath.Join(root, "go.mod"), "module m\n")
	writeGoFile(t, filepath.Join(root, "sub", "a.go"), "package sub\n\nfunc A() {}\n")
	writeGoFile(t, filepath.Join(root, "other.go"), "package m\n\nfunc B() {}\n")

	gen := &fakeCoverage{profilePath: filepath.Join(root, "missing.out")}
	app, stdout, _ := newTestApp(t, root, gen)
	code := app.Execute(context.Background(), []string{"sub"})
	if code != ExitOK {
		t.Fatalf("expected exit 0, got %d", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "A ") {
		t.Fatalf("expected function A from sub/ in output: %q", out)
	}
	if strings.Contains(out, "B ") {
		t.Fatalf("did not expect B (outside sub/) in output: %q", out)
	}
}

func TestCoverageGenerateErr(t *testing.T) {
	canceled := coverageGenerateErr("mod", context.Canceled)
	if canceled == nil || !strings.Contains(canceled.Error(), "interrupted") {
		t.Fatalf("expected interrupted wrap for Canceled, got %v", canceled)
	}
	deadline := coverageGenerateErr("mod", context.DeadlineExceeded)
	if deadline == nil || !strings.Contains(deadline.Error(), "interrupted") {
		t.Fatalf("expected interrupted wrap for DeadlineExceeded, got %v", deadline)
	}
	other := errors.New("boom")
	if got := coverageGenerateErr("mod", other); !errors.Is(got, other) {
		t.Fatalf("expected passthrough of other errors, got %v", got)
	}
}

func setupTwoModules(t *testing.T) (root, moda, modb string) {
	t.Helper()
	root = t.TempDir()
	moda = filepath.Join(root, "a")
	modb = filepath.Join(root, "b")
	writeGoFile(t, filepath.Join(moda, "go.mod"), "module a\n")
	writeGoFile(t, filepath.Join(modb, "go.mod"), "module b\n")
	// Distinct names so report order assertions are unambiguous.
	writeGoFile(t, filepath.Join(moda, "alpha.go"), "package a\n\nfunc Alpha() {}\n")
	writeGoFile(t, filepath.Join(modb, "beta.go"), "package b\n\nfunc Beta() {}\n")
	return root, moda, modb
}

func TestExecute_MultiModulePreservesFirstSeenOrder(t *testing.T) {
	root, moda, modb := setupTwoModules(t)
	missing := filepath.Join(root, "missing.out")
	// Make module a slower so completion order would be b-then-a if merge were
	// wrong; first-seen order after sorted discovery is still a-then-b.
	inner := &fakeCoverage{
		perModule: map[string]fakeModuleResult{
			moda: {profilePath: missing},
			modb: {profilePath: missing},
		},
	}
	gen := &slowFirstModule{inner: inner, slowRoot: moda, delay: 80 * time.Millisecond}
	app, stdout, _ := newTestApp(t, root, gen)
	code := app.Execute(context.Background(), []string{
		"--max-workers", "2",
		"a", "b",
	})
	if code != ExitOK {
		t.Fatalf("expected exit 0, got %d", code)
	}
	out := stdout.String()
	alphaIdx := strings.Index(out, "Alpha")
	betaIdx := strings.Index(out, "Beta")
	if alphaIdx < 0 || betaIdx < 0 {
		t.Fatalf("expected both Alpha and Beta in output: %q", out)
	}
	// Sorted discovery visits a/ before b/, so first-seen module order is a, b.
	if alphaIdx > betaIdx {
		t.Fatalf("expected Alpha (module a, first-seen) before Beta despite slower coverage; got %q", out)
	}
}

// slowFirstModule delays Generate for one module root to force completion-order
// inversion relative to first-seen merge order.
type slowFirstModule struct {
	inner    *fakeCoverage
	slowRoot string
	delay    time.Duration
}

func (s *slowFirstModule) Generate(ctx context.Context, moduleRoot string) (string, bool, error) {
	if moduleRoot == s.slowRoot && s.delay > 0 {
		timer := time.NewTimer(s.delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return "", false, ctx.Err()
		}
	}
	return s.inner.Generate(ctx, moduleRoot)
}

func TestExecute_ParallelCoverageOverlapsAndRespectsWorkerLimit(t *testing.T) {
	root, moda, modb := setupTwoModules(t)
	modc := filepath.Join(root, "c")
	writeGoFile(t, filepath.Join(modc, "go.mod"), "module c\n")
	writeGoFile(t, filepath.Join(modc, "gamma.go"), "package c\n\nfunc Gamma() {}\n")

	missing := filepath.Join(root, "missing.out")
	gen := &fakeCoverage{
		delay: 80 * time.Millisecond,
		perModule: map[string]fakeModuleResult{
			moda: {profilePath: missing},
			modb: {profilePath: missing},
			modc: {profilePath: missing},
		},
	}
	app, _, _ := newTestApp(t, root, gen)
	code := app.Execute(context.Background(), []string{
		"--max-workers", "2",
		"a", "b", "c",
	})
	if code != ExitOK {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if gen.callCount() != 3 {
		t.Fatalf("expected 3 coverage calls, got %d", gen.callCount())
	}
	max := atomic.LoadInt32(&gen.maxInFlight)
	if max < 2 {
		t.Fatalf("expected concurrent overlap (maxInFlight>=2), got %d", max)
	}
	if max > 2 {
		t.Fatalf("expected worker limit of 2, got maxInFlight %d", max)
	}
}

func TestExecute_ModuleFailureCancelsSiblings(t *testing.T) {
	root, moda, modb := setupTwoModules(t)
	missing := filepath.Join(root, "missing.out")

	block, release := context.WithCancel(context.Background())
	gen := &fakeCoverage{
		blockUntil: block,
		perModule: map[string]fakeModuleResult{
			moda: {err: errors.New("coverage boom")},
			modb: {profilePath: missing},
		},
	}
	app, _, stderr := newTestApp(t, root, gen)

	done := make(chan int, 1)
	go func() {
		done <- app.Execute(context.Background(), []string{
			"--max-workers", "2",
			"a", "b",
		})
	}()

	// Wait until both modules have entered Generate, then let the failing one proceed.
	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt32(&gen.inFlight) < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if atomic.LoadInt32(&gen.inFlight) < 2 {
		t.Fatal("timed out waiting for both modules to start Generate")
	}
	release()

	code := <-done
	if code != ExitUsageError {
		t.Fatalf("expected exit 1, got %d (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "coverage boom") {
		t.Fatalf("expected initiating error in stderr, got %q", stderr.String())
	}
}

func TestExecute_MultiModuleWarningsEmitInModuleOrder(t *testing.T) {
	root, moda, modb := setupTwoModules(t)
	missingA := filepath.Join(root, "missing-a.out")
	missingB := filepath.Join(root, "missing-b.out")
	gen := &fakeCoverage{
		perModule: map[string]fakeModuleResult{
			moda: {profilePath: missingA, testsFailed: true},
			modb: {profilePath: missingB},
		},
	}
	app, _, stderr := newTestApp(t, root, gen)
	code := app.Execute(context.Background(), []string{
		"--max-workers", "2",
		filepath.Join("a", "alpha.go"),
		filepath.Join("b", "beta.go"),
	})
	if code != ExitOK {
		t.Fatalf("expected exit 0, got %d", code)
	}
	errOut := stderr.String()
	failIdx := strings.Index(errOut, "reported failures in "+moda)
	naIdx := strings.Index(errOut, "coverage profile not found for "+moda)
	naB := strings.Index(errOut, "coverage profile not found for "+modb)
	if failIdx < 0 || naIdx < 0 || naB < 0 {
		t.Fatalf("expected ordered warnings for both modules, got %q", errOut)
	}
	if failIdx >= naIdx || naIdx >= naB {
		t.Fatalf("expected moda warnings before modb N/A warning, got %q", errOut)
	}
}
