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
