package report

import (
	"fmt"
	"path/filepath"
	"strings"

	"go-check-it/internal/practices"
)

// FormatFindings renders practice findings as a human-readable list. When
// explain is true, each finding is followed by its full rationale, fix, and
// doc link; otherwise each finding is a single line.
func FormatFindings(findings []practices.Finding, relativeTo string, explain bool) string {
	var b strings.Builder
	b.WriteString("Practice Findings\n")
	b.WriteString("=================\n")
	if len(findings) == 0 {
		b.WriteString("(none)\n")
		return b.String()
	}

	for _, f := range findings {
		loc := findingLocation(f, relativeTo)
		fmt.Fprintf(&b, "%s: [%s] %s", loc, f.Rule, f.Message)
		if f.Confidence < 10 {
			fmt.Fprintf(&b, " (confidence %d/10)", f.Confidence)
		}
		b.WriteByte('\n')
		if explain {
			fmt.Fprintf(&b, "    why: %s\n", f.Description.Why)
			fmt.Fprintf(&b, "    fix: %s\n", f.Description.Fix)
			fmt.Fprintf(&b, "    doc: %s\n", f.Description.DocURL)
		}
	}
	return b.String()
}

func findingLocation(f practices.Finding, relativeTo string) string {
	file := f.Position.Filename
	if relativeTo != "" {
		if rel, err := filepath.Rel(relativeTo, file); err == nil {
			file = rel
		}
	}
	return fmt.Sprintf("%s:%d", file, f.Position.Line)
}
