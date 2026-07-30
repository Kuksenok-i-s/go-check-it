package report

import (
	"encoding/json"
	"path/filepath"

	"go-check-it/internal/analyzer"
	"go-check-it/internal/practices"
)

// DefaultAgentTopN is the default number of CRAP hotspots included in
// --format=agent-json when --top is omitted.
const DefaultAgentTopN = 6

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

// agentJSONReport is a bounded machine report for LLM agents: summary counts,
// truncated CRAP hotspots, and practice findings with full explanations.
type agentJSONReport struct {
	Summary  agentJSONSummary `json:"summary"`
	Hotspots []jsonFunction   `json:"hotspots"`
	Findings []jsonFinding    `json:"findings"`
	Omitted  agentJSONOmitted `json:"omitted"`
}

type agentJSONSummary struct {
	Threshold           float64 `json:"threshold"`
	MaxCrap             float64 `json:"maxCrap"`
	MaxCrapFunction     string  `json:"maxCrapFunction,omitempty"`
	MaxCrapFile         string  `json:"maxCrapFile,omitempty"`
	MaxCrapLine         int     `json:"maxCrapLine,omitempty"`
	FunctionCount       int     `json:"functionCount"`
	AboveThresholdCount int     `json:"aboveThresholdCount"`
	FindingCount        int     `json:"findingCount"`
	HotspotLimit        int     `json:"hotspotLimit"`
}

type agentJSONOmitted struct {
	FunctionsAboveHotspots int `json:"functionsAboveHotspots"`
	FunctionsTotalOmitted  int `json:"functionsTotalOmitted"`
}

// FormatJSON renders metrics and findings as a single JSON document.
func FormatJSON(metrics []analyzer.FunctionMetrics, findings []practices.Finding, relativeTo string) (string, error) {
	out := jsonReport{
		Functions: toJSONFunctions(metrics, relativeTo),
		Findings:  toJSONFindings(findings, relativeTo),
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// FormatAgentJSON renders a bounded agent report: summary, hotspots (union of
// above-threshold and top-N), all practice findings, and omitted counts.
// Exit decisions must still use the complete analysis, not this truncation.
func FormatAgentJSON(metrics []analyzer.FunctionMetrics, findings []practices.Finding, relativeTo string, threshold float64, topN int) (string, error) {
	if topN < 1 {
		topN = DefaultAgentTopN
	}
	hotspots := Hotspots(metrics, threshold, topN)
	above := CountAboveThreshold(metrics, threshold)
	summary := agentJSONSummary{
		Threshold:           threshold,
		MaxCrap:             MaxCrap(metrics),
		FunctionCount:       len(metrics),
		AboveThresholdCount: above,
		FindingCount:        len(findings),
		HotspotLimit:        topN,
	}
	if best := MaxCrapFunction(metrics); best != nil {
		summary.MaxCrapFunction = best.FunctionName
		summary.MaxCrapFile = relPath(best.File, relativeTo)
		summary.MaxCrapLine = best.Line
	}

	out := agentJSONReport{
		Summary:  summary,
		Hotspots: toJSONFunctions(hotspots, relativeTo),
		Findings: toJSONFindings(findings, relativeTo),
		Omitted: agentJSONOmitted{
			FunctionsAboveHotspots: max(0, above-len(hotspots)),
			FunctionsTotalOmitted:  max(0, len(metrics)-len(hotspots)),
		},
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func toJSONFunctions(metrics []analyzer.FunctionMetrics, relativeTo string) []jsonFunction {
	out := make([]jsonFunction, 0, len(metrics))
	for _, m := range metrics {
		out = append(out, jsonFunction{
			FunctionName:    m.FunctionName,
			Package:         m.Package,
			File:            relPath(m.File, relativeTo),
			Line:            m.Line,
			Complexity:      m.Complexity,
			CoveragePercent: m.CoveragePercent,
			CrapScore:       m.CrapScore,
		})
	}
	return out
}

func toJSONFindings(findings []practices.Finding, relativeTo string) []jsonFinding {
	out := make([]jsonFinding, 0, len(findings))
	for _, f := range findings {
		out = append(out, jsonFinding{
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
	return out
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
