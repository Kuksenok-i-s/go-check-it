// Package report renders CRAP metrics as a tabular report, mirroring
// crap4java's ReportFormatter.
package report

import (
	"fmt"
	"path/filepath"
	"strings"

	"go-check-it/internal/analyzer"
)

// Format renders entries as a fixed-width table sorted by CRAP score
// descending, with unknown (N/A) scores last.
func Format(entries []analyzer.FunctionMetrics, relativeTo string) string {
	header := fmt.Sprintf("%-28s %-16s %4s %7s %8s  %s", "Function", "Package", "CC", "Cov%", "CRAP", "Location")
	var b strings.Builder
	b.WriteString("CRAP Report\n")
	b.WriteString("===========\n")
	b.WriteString(header)
	b.WriteByte('\n')
	b.WriteString(strings.Repeat("-", len(header)))
	b.WriteByte('\n')

	for _, e := range entries {
		fmt.Fprintf(&b, "%-28s %-16s %4d %7s %8s  %s\n",
			e.FunctionName,
			e.Package,
			e.Complexity,
			formatCoverage(e.CoveragePercent),
			formatCrap(e.CrapScore),
			location(e, relativeTo),
		)
	}
	return b.String()
}

func formatCoverage(coverage *float64) string {
	if coverage == nil {
		return "  N/A "
	}
	return fmt.Sprintf("%5.1f%%", *coverage)
}

func formatCrap(score *float64) string {
	if score == nil {
		return "     N/A"
	}
	return fmt.Sprintf("%8.1f", *score)
}

func location(e analyzer.FunctionMetrics, relativeTo string) string {
	file := e.File
	if relativeTo != "" {
		if rel, err := filepath.Rel(relativeTo, e.File); err == nil {
			file = rel
		}
	}
	return fmt.Sprintf("%s:%d", file, e.Line)
}

// MaxCrap returns the largest numeric CRAP score in metrics, or 0 if none
// are numeric.
func MaxCrap(metrics []analyzer.FunctionMetrics) float64 {
	max := 0.0
	for _, m := range metrics {
		if m.CrapScore != nil && *m.CrapScore > max {
			max = *m.CrapScore
		}
	}
	return max
}

// CountAboveThreshold returns how many metrics have a numeric CRAP score
// strictly greater than threshold.
func CountAboveThreshold(metrics []analyzer.FunctionMetrics, threshold float64) int {
	n := 0
	for _, m := range metrics {
		if m.CrapScore != nil && *m.CrapScore > threshold {
			n++
		}
	}
	return n
}

// MaxCrapFunction returns the metrics row with the highest numeric CRAP
// score, or nil when no numeric scores exist. Ties keep the first occurrence
// (metrics are expected to be sorted CRAP-descending).
func MaxCrapFunction(metrics []analyzer.FunctionMetrics) *analyzer.FunctionMetrics {
	var best *analyzer.FunctionMetrics
	max := 0.0
	for i := range metrics {
		m := &metrics[i]
		if m.CrapScore == nil {
			continue
		}
		if best == nil || *m.CrapScore > max {
			best = m
			max = *m.CrapScore
		}
	}
	return best
}

// TopN returns up to n metrics with non-nil CRAP scores, preserving input
// order. Metrics are expected to already be sorted CRAP-descending. When n
// is less than 1, an empty slice is returned.
func TopN(metrics []analyzer.FunctionMetrics, n int) []analyzer.FunctionMetrics {
	if n < 1 {
		return nil
	}
	out := make([]analyzer.FunctionMetrics, 0, n)
	for _, m := range metrics {
		if m.CrapScore == nil {
			continue
		}
		out = append(out, m)
		if len(out) >= n {
			break
		}
	}
	return out
}

// Hotspots returns the union of metrics above threshold and the global top-N
// by CRAP, deduplicated by file:line:name and preserving CRAP-descending
// order. Nil CRAP scores are excluded.
func Hotspots(metrics []analyzer.FunctionMetrics, threshold float64, topN int) []analyzer.FunctionMetrics {
	seen := make(map[string]bool)
	out := make([]analyzer.FunctionMetrics, 0)
	add := func(m analyzer.FunctionMetrics) {
		if m.CrapScore == nil {
			return
		}
		key := fmt.Sprintf("%s:%d:%s", m.File, m.Line, m.FunctionName)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, m)
	}
	for _, m := range metrics {
		if m.CrapScore != nil && *m.CrapScore > threshold {
			add(m)
		}
	}
	for _, m := range TopN(metrics, topN) {
		add(m)
	}
	return out
}
