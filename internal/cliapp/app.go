// Package cliapp wires together file selection, coverage generation, and
// analysis into the go-check-it command-line application, mirroring
// crap4java's CliApplication.
package cliapp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"go-check-it/internal/analyzer"
	"go-check-it/internal/changed"
	"go-check-it/internal/coverage"
	"go-check-it/internal/modroot"
	"go-check-it/internal/practices"
	"go-check-it/internal/report"
	"go-check-it/internal/sourcefind"
)

// Threshold is the CRAP score above which the run is considered a failure.
const Threshold = 8.0

// Exit codes, mirroring crap4java's Main/CliApplication, plus
// ExitFindingsFailed for the opt-in --fail-on-findings flag.
const (
	ExitOK                = 0
	ExitUsageError        = 1
	ExitThresholdExceeded = 2
	ExitFindingsFailed    = 3
)

// CoverageGenerator produces a coverage profile for a module. It is declared
// here, in the package that consumes it, rather than in package coverage
// (which implements it) — see the Go Code Review Comments "Interfaces"
// guideline.
//
// Generate may be called concurrently for distinct module roots when the
// implementation supports it (the real coverage.Runner does).
type CoverageGenerator interface {
	Generate(ctx context.Context, moduleRoot string) (profilePath string, testsFailed bool, err error)
}

// App is the go-check-it CLI application.
type App struct {
	ProjectRoot string
	Stdout      io.Writer
	Stderr      io.Writer
	Coverage    CoverageGenerator
}

// New returns an App wired to the real `go test` coverage runner.
func New(projectRoot string, stdout, stderr io.Writer) *App {
	return &App{
		ProjectRoot: projectRoot,
		Stdout:      stdout,
		Stderr:      stderr,
		Coverage:    coverage.NewRunner(),
	}
}

// Execute runs the tool for the given CLI arguments and returns a process
// exit code. ctx governs the run: if it is canceled (e.g. via Ctrl-C) while
// coverage is being generated, the underlying `go test` process is killed
// and Execute returns promptly instead of hanging.
func (a *App) Execute(ctx context.Context, args []string) int {
	parsed, err := ParseArgs(args)
	if err != nil {
		fmt.Fprintln(a.Stderr, err)
		fmt.Fprint(a.Stdout, Usage())
		return ExitUsageError
	}
	if parsed.Mode == ModeHelp {
		fmt.Fprint(a.Stdout, Usage())
		return ExitOK
	}
	return a.runAnalysis(ctx, parsed)
}

func (a *App) runAnalysis(ctx context.Context, parsed Arguments) int {
	files, err := a.filesForMode(ctx, parsed)
	if err != nil {
		fmt.Fprintln(a.Stderr, err)
		return ExitUsageError
	}
	if len(files) == 0 {
		fmt.Fprintln(a.Stdout, "No Go files to analyze.")
		return ExitOK
	}

	metrics, findings, err := a.collectResults(ctx, files, parsed.MaxWorkers)
	if err != nil {
		fmt.Fprintln(a.Stderr, err)
		return ExitUsageError
	}
	if err := a.writeReport(metrics, findings, parsed); err != nil {
		fmt.Fprintln(a.Stderr, err)
		return ExitUsageError
	}
	return a.exitAfterReport(report.MaxCrap(metrics), findings, parsed)
}

func (a *App) collectResults(ctx context.Context, files []string, maxWorkers int) ([]analyzer.FunctionMetrics, []practices.Finding, error) {
	metrics, err := a.analyzeByModule(ctx, files, maxWorkers)
	if err != nil {
		return nil, nil, err
	}
	findings, err := practices.Run(files)
	if err != nil {
		return nil, nil, err
	}
	return metrics, findings, nil
}

func (a *App) exitAfterReport(max float64, findings []practices.Finding, parsed Arguments) int {
	if max > Threshold {
		fmt.Fprintf(a.Stderr, "CRAP threshold exceeded: %.1f > %.1f\n", max, Threshold)
		return ExitThresholdExceeded
	}
	if parsed.FailOnFindings && len(findings) > 0 {
		fmt.Fprintf(a.Stderr, "%d practice finding(s) reported with --fail-on-findings\n", len(findings))
		return ExitFindingsFailed
	}
	return ExitOK
}

func (a *App) writeReport(metrics []analyzer.FunctionMetrics, findings []practices.Finding, parsed Arguments) error {
	if parsed.Format == formatJSON {
		out, err := report.FormatJSON(metrics, findings, a.ProjectRoot)
		if err != nil {
			return err
		}
		fmt.Fprintln(a.Stdout, out)
		return nil
	}
	fmt.Fprint(a.Stdout, report.Format(metrics, a.ProjectRoot))
	fmt.Fprint(a.Stdout, report.FormatFindings(findings, a.ProjectRoot, parsed.Explain))
	return nil
}

func (a *App) filesForMode(ctx context.Context, parsed Arguments) ([]string, error) {
	switch parsed.Mode {
	case ModeAllSrc:
		return sourcefind.FindAllGoFiles(a.ProjectRoot)
	case ModeChangedSrc:
		return changed.ChangedGoFiles(ctx, a.ProjectRoot)
	case ModeExplicitFiles:
		return a.explicitFiles(parsed.FileArgs)
	default:
		return nil, nil
	}
}

func (a *App) explicitFiles(args []string) ([]string, error) {
	seen := make(map[string]bool)
	var files []string
	for _, arg := range args {
		found, err := a.expandArg(arg)
		if err != nil {
			return nil, err
		}
		for _, f := range found {
			if !seen[f] {
				seen[f] = true
				files = append(files, f)
			}
		}
	}
	sort.Strings(files)
	return files, nil
}

func (a *App) expandArg(arg string) ([]string, error) {
	path := arg
	if !filepath.IsAbs(path) {
		path = filepath.Join(a.ProjectRoot, path)
	}
	path = filepath.Clean(path)

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("cannot access %s: %w", arg, err)
	}
	if info.IsDir() {
		return sourcefind.FindAllGoFiles(path)
	}
	return []string{path}, nil
}

// moduleResult holds one module's analysis outcome for ordered merge.
type moduleResult struct {
	metrics  []analyzer.FunctionMetrics
	warnings []string
	err      error
}

func (a *App) analyzeByModule(ctx context.Context, files []string, maxWorkers int) ([]analyzer.FunctionMetrics, error) {
	order, groups := modroot.GroupByModuleRoot(a.ProjectRoot, files)
	if len(order) == 0 {
		return nil, nil
	}
	results, err := a.runModulePool(ctx, order, groups, clampWorkers(maxWorkers, len(order)))
	if err != nil {
		return nil, err
	}
	return mergeModuleResults(order, results, a.Stderr), nil
}

func clampWorkers(maxWorkers, moduleCount int) int {
	workers := maxWorkers
	if workers > moduleCount {
		workers = moduleCount
	}
	if workers < 1 {
		return 1
	}
	return workers
}

func (a *App) runModulePool(ctx context.Context, order []string, groups map[string][]string, workers int) ([]moduleResult, error) {
	results := make([]moduleResult, len(order))
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobs := make(chan int)
	var wg sync.WaitGroup
	var firstErrMu sync.Mutex
	var firstErr error

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				a.runModuleJob(runCtx, cancel, order, groups, results, idx, &firstErrMu, &firstErr)
			}
		}()
	}
	for i := range order {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return results, firstErr
}

func (a *App) runModuleJob(
	ctx context.Context,
	cancel context.CancelFunc,
	order []string,
	groups map[string][]string,
	results []moduleResult,
	idx int,
	firstErrMu *sync.Mutex,
	firstErr *error,
) {
	if ctx.Err() != nil {
		return
	}
	metrics, warnings, err := a.analyzeOneModule(ctx, order[idx], groups[order[idx]])
	results[idx] = moduleResult{metrics: metrics, warnings: warnings, err: err}
	if err == nil {
		return
	}
	firstErrMu.Lock()
	defer firstErrMu.Unlock()
	if *firstErr == nil {
		*firstErr = err
		cancel()
	}
}

func mergeModuleResults(order []string, results []moduleResult, stderr io.Writer) []analyzer.FunctionMetrics {
	var all []analyzer.FunctionMetrics
	for i := range order {
		for _, w := range results[i].warnings {
			fmt.Fprintln(stderr, w)
		}
		all = append(all, results[i].metrics...)
	}
	return all
}

func (a *App) analyzeOneModule(ctx context.Context, moduleRoot string, files []string) ([]analyzer.FunctionMetrics, []string, error) {
	index, warnings, err := a.coverageIndexFor(ctx, moduleRoot)
	if err != nil {
		return nil, warnings, err
	}
	metrics, err := analyzer.Analyze(files, index)
	if err != nil {
		return nil, warnings, err
	}
	return metrics, warnings, nil
}

func (a *App) coverageIndexFor(ctx context.Context, moduleRoot string) (coverage.Index, []string, error) {
	profilePath, testsFailed, err := a.Coverage.Generate(ctx, moduleRoot)
	if err != nil {
		if profilePath != "" {
			_ = os.Remove(profilePath)
		}
		return nil, nil, coverageGenerateErr(moduleRoot, err)
	}
	// Best-effort cleanup of the temporary profile; a leftover file in the
	// OS temp directory is harmless and not worth failing the run over.
	defer os.Remove(profilePath)

	var warnings []string
	if testsFailed {
		warnings = append(warnings, "Warning: `go test` reported failures in "+moduleRoot+"; coverage may be incomplete.")
	}
	if !usableProfile(profilePath) {
		warnings = append(warnings, "Warning: coverage profile not found for "+moduleRoot+". Coverage will be N/A.")
		return nil, warnings, nil
	}
	index, err := coverage.ParseProfile(profilePath, moduleRoot)
	return index, warnings, err
}

func coverageGenerateErr(moduleRoot string, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("coverage generation for %s was interrupted: %w", moduleRoot, err)
	}
	return err
}

func usableProfile(profilePath string) bool {
	info, err := os.Stat(profilePath)
	return err == nil && info.Size() > 0
}
