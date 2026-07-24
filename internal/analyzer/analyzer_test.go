package analyzer

import (
	"os"
	"path/filepath"
	"testing"

	"go-check-it/internal/coverage"
)

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAnalyze_NoCoverageYieldsNilCrap(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "sample.go")
	writeFile(t, file, `package sample

func Risky(a int) int {
	if a > 0 {
		return a
	}
	return -a
}
`)

	metrics, err := Analyze([]string{file}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(metrics))
	}
	m := metrics[0]
	if m.FunctionName != "Risky" || m.Complexity != 2 {
		t.Fatalf("unexpected metric: %+v", m)
	}
	if m.CoveragePercent != nil || m.CrapScore != nil {
		t.Fatalf("expected nil coverage/crap without an index, got %+v", m)
	}
}

func TestAnalyze_WithFullCoverageCrapEqualsComplexity(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "sample.go")
	writeFile(t, file, `package sample

func Covered(a int) int {
	if a > 0 {
		return a
	}
	return -a
}
`)

	index := coverage.Index{
		file: {
			{StartLine: 3, EndLine: 8, NumStmt: 4, Count: 1},
		},
	}

	metrics, err := Analyze([]string{file}, index)
	if err != nil {
		t.Fatal(err)
	}
	m := metrics[0]
	if m.CoveragePercent == nil || *m.CoveragePercent != 100 {
		t.Fatalf("expected 100%% coverage, got %+v", m.CoveragePercent)
	}
	if m.CrapScore == nil || *m.CrapScore != float64(m.Complexity) {
		t.Fatalf("expected crap == complexity at full coverage, got %+v", m.CrapScore)
	}
}

func TestAnalyze_SortsByCrapDescendingWithNilsLast(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "sample.go")
	writeFile(t, file, `package sample

func Simple() int {
	return 1
}

func Complex(a, b, c int) int {
	if a > 0 && b > 0 {
		if c > 0 {
			return a
		}
	}
	for i := 0; i < a; i++ {
		b++
	}
	return b
}
`)

	index := coverage.Index{
		file: {
			{StartLine: 3, EndLine: 5, NumStmt: 1, Count: 1},
		},
	}

	metrics, err := Analyze([]string{file}, index)
	if err != nil {
		t.Fatal(err)
	}
	if len(metrics) != 2 {
		t.Fatalf("expected 2 metrics, got %d", len(metrics))
	}
	// Complex has no overlapping coverage blocks, so its CRAP score is nil
	// and it must sort after Simple's numeric score, regardless of magnitude.
	if metrics[0].FunctionName != "Simple" || metrics[0].CrapScore == nil {
		t.Fatalf("expected Simple (numeric CRAP) first, got %+v", metrics)
	}
	if metrics[1].FunctionName != "Complex" || metrics[1].CrapScore != nil {
		t.Fatalf("expected Complex (nil CRAP) last, got %+v", metrics)
	}
}

func TestAnalyze_MissingFileIsSkipped(t *testing.T) {
	metrics, err := Analyze([]string{"/does/not/exist.go"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(metrics) != 0 {
		t.Fatalf("expected no metrics, got %+v", metrics)
	}
}
