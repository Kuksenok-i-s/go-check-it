// Package analyzer combines complexity and coverage data into CRAP metrics
// for a set of Go source files, mirroring crap4java's CrapAnalyzer.
package analyzer

import (
	"os"
	"sort"

	"go-check-it/internal/complexity"
	"go-check-it/internal/coverage"
	"go-check-it/internal/crap"
)

// FunctionMetrics is a single report row: a function's identity, complexity,
// coverage, and CRAP score.
type FunctionMetrics struct {
	FunctionName    string
	Package         string
	File            string
	Line            int
	Complexity      int
	CoveragePercent *float64
	CrapScore       *float64
}

// Analyze parses each file in files and attributes coverage from index (which
// may be nil/empty when coverage is unavailable), returning one
// FunctionMetrics per concrete function/method found.
func Analyze(files []string, index coverage.Index) ([]FunctionMetrics, error) {
	var metrics []FunctionMetrics
	for _, file := range files {
		if _, err := os.Stat(file); err != nil {
			continue
		}
		funcs, err := complexity.ParseFile(file)
		if err != nil {
			return nil, err
		}
		for _, fn := range funcs {
			coveragePct := coveragePercent(index, fn)
			score := crap.Calculate(fn.Complexity, coveragePct)
			metrics = append(metrics, FunctionMetrics{
				FunctionName:    fn.Name,
				Package:         fn.Package,
				File:            fn.File,
				Line:            fn.StartLine,
				Complexity:      fn.Complexity,
				CoveragePercent: coveragePct,
				CrapScore:       score,
			})
		}
	}
	sortByCrapDescending(metrics)
	return metrics, nil
}

func coveragePercent(index coverage.Index, fn complexity.FuncDescriptor) *float64 {
	if index == nil {
		return nil
	}
	total, covered := coverage.FunctionCoverage(index, fn.File, fn.StartLine, fn.EndLine)
	if total == 0 {
		return nil
	}
	pct := (float64(covered) * 100.0) / float64(total)
	return &pct
}

// sortByCrapDescending stably sorts metrics by CRAP score descending, with
// unknown (nil) scores last, mirroring ReportFormatter's ordering.
func sortByCrapDescending(metrics []FunctionMetrics) {
	sort.SliceStable(metrics, func(i, j int) bool {
		a, b := metrics[i].CrapScore, metrics[j].CrapScore
		if a == nil && b == nil {
			return false
		}
		if a == nil {
			return false
		}
		if b == nil {
			return true
		}
		return *a > *b
	})
}
