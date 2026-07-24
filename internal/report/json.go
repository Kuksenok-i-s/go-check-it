package report

import (
	"encoding/json"
	"path/filepath"

	"go-check-it/internal/analyzer"
	"go-check-it/internal/practices"
)

// jsonReport is go-check-it's machine-readable output: CRAP metrics plus
// practice findings, each finding carrying its full explanation so an LLM
// agent consuming this JSON never needs a side lookup.
type jsonReport struct {
	Functions []jsonFunction `json:"functions"`
	Findings  []jsonFinding  `json:"findings"`
}

type jsonFunction struct {
	FunctionName    string   `json:"functionName"`
	Package         string   `json:"package"`
	File            string   `json:"file"`
	Line            int      `json:"line"`
	Complexity      int      `json:"complexity"`
	CoveragePercent *float64 `json:"coveragePercent"`
	CrapScore       *float64 `json:"crapScore"`
}

type jsonFinding struct {
	Rule       string `json:"rule"`
	Message    string `json:"message"`
	File       string `json:"file"`
	Line       int    `json:"line"`
	Column     int    `json:"column"`
	Confidence int    `json:"confidence"`
	Summary    string `json:"summary"`
	Why        string `json:"why"`
	Fix        string `json:"fix"`
	DocURL     string `json:"docUrl"`
}

// FormatJSON renders metrics and findings as a single JSON document.
func FormatJSON(metrics []analyzer.FunctionMetrics, findings []practices.Finding, relativeTo string) (string, error) {
	out := jsonReport{
		Functions: make([]jsonFunction, 0, len(metrics)),
		Findings:  make([]jsonFinding, 0, len(findings)),
	}
	for _, m := range metrics {
		out.Functions = append(out.Functions, jsonFunction{
			FunctionName:    m.FunctionName,
			Package:         m.Package,
			File:            relPath(m.File, relativeTo),
			Line:            m.Line,
			Complexity:      m.Complexity,
			CoveragePercent: m.CoveragePercent,
			CrapScore:       m.CrapScore,
		})
	}
	for _, f := range findings {
		out.Findings = append(out.Findings, jsonFinding{
			Rule:       f.Rule,
			Message:    f.Message,
			File:       relPath(f.Position.Filename, relativeTo),
			Line:       f.Position.Line,
			Column:     f.Position.Column,
			Confidence: f.Confidence,
			Summary:    f.Description.Summary,
			Why:        f.Description.Why,
			Fix:        f.Description.Fix,
			DocURL:     f.Description.DocURL,
		})
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func relPath(file, relativeTo string) string {
	if relativeTo == "" {
		return file
	}
	if rel, err := filepath.Rel(relativeTo, file); err == nil {
		return rel
	}
	return file
}
