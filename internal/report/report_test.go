package report

import (
	"encoding/json"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"go-check-it/internal/analyzer"
	"go-check-it/internal/practices"
)

func f64(v float64) *float64 { return &v }

func TestFormat_ContainsHeaderAndRows(t *testing.T) {
	entries := []analyzer.FunctionMetrics{
		{FunctionName: "Risky", Package: "pkg", File: "pkg/risky.go", Line: 10, Complexity: 5, CoveragePercent: f64(20), CrapScore: f64(30.5)},
		{FunctionName: "Unknown", Package: "pkg", File: "pkg/unknown.go", Line: 4, Complexity: 2, CoveragePercent: nil, CrapScore: nil},
	}
	out := Format(entries, "")
	if !strings.Contains(out, "CRAP Report") {
		t.Fatalf("missing title: %q", out)
	}
	if !strings.Contains(out, "Risky") || !strings.Contains(out, "30.5") {
		t.Fatalf("missing Risky row: %q", out)
	}
	if !strings.Contains(out, "N/A") {
		t.Fatalf("missing N/A row: %q", out)
	}
}

func TestMaxCrap_IgnoresNilsAndTracksMax(t *testing.T) {
	metrics := []analyzer.FunctionMetrics{
		{CrapScore: f64(3.0)},
		{CrapScore: nil},
		{CrapScore: f64(9.5)},
	}
	if got := MaxCrap(metrics); got != 9.5 {
		t.Fatalf("expected 9.5, got %v", got)
	}
}

func TestMaxCrap_EmptyOrAllNilIsZero(t *testing.T) {
	if got := MaxCrap(nil); got != 0 {
		t.Fatalf("expected 0, got %v", got)
	}
	if got := MaxCrap([]analyzer.FunctionMetrics{{CrapScore: nil}}); got != 0 {
		t.Fatalf("expected 0, got %v", got)
	}
}

func TestFormatFindings_EmptyAndExplained(t *testing.T) {
	empty := FormatFindings(nil, "", false)
	if !strings.Contains(empty, "(none)") {
		t.Fatalf("expected empty marker, got %q", empty)
	}

	root := "/proj"
	finding := practices.Finding{
		Rule:    "contextprop",
		Message: "use CommandContext",
		Position: token.Position{
			Filename: filepath.Join(root, "internal/changed/changed.go"),
			Line:     20,
			Column:   9,
		},
		Description: practices.Description{
			Summary: "Propagate context",
			Why:     "callers cannot cancel",
			Fix:     "use CommandContext",
			DocURL:  "https://go.dev/blog/context",
		},
		Confidence: 7,
	}
	out := FormatFindings([]practices.Finding{finding}, root, true)
	if !strings.Contains(out, "internal/changed/changed.go:20") {
		t.Fatalf("missing relative location: %q", out)
	}
	if !strings.Contains(out, "[contextprop]") || !strings.Contains(out, "use CommandContext") {
		t.Fatalf("missing finding line: %q", out)
	}
	if !strings.Contains(out, "(confidence 7/10)") {
		t.Fatalf("missing confidence: %q", out)
	}
	if !strings.Contains(out, "why: callers cannot cancel") ||
		!strings.Contains(out, "fix: use CommandContext") ||
		!strings.Contains(out, "doc: https://go.dev/blog/context") {
		t.Fatalf("missing explain block: %q", out)
	}
}

func TestCountAboveThresholdAndTopN(t *testing.T) {
	metrics := []analyzer.FunctionMetrics{
		{FunctionName: "A", File: "a.go", Line: 1, CrapScore: f64(30)},
		{FunctionName: "B", File: "b.go", Line: 2, CrapScore: f64(10)},
		{FunctionName: "C", File: "c.go", Line: 3, CrapScore: f64(5)},
		{FunctionName: "D", File: "d.go", Line: 4, CrapScore: nil},
	}
	if got := CountAboveThreshold(metrics, 8); got != 2 {
		t.Fatalf("CountAboveThreshold: got %d, want 2", got)
	}
	top := TopN(metrics, 2)
	if len(top) != 2 || top[0].FunctionName != "A" || top[1].FunctionName != "B" {
		t.Fatalf("TopN unexpected: %#v", top)
	}
	if TopN(metrics, 0) != nil {
		t.Fatal("TopN(0) should be empty")
	}
	best := MaxCrapFunction(metrics)
	if best == nil || best.FunctionName != "A" {
		t.Fatalf("MaxCrapFunction: %#v", best)
	}
	hot := Hotspots(metrics, 8, 1)
	// Above threshold: A,B; top-1: A; union still A,B in order.
	if len(hot) != 2 || hot[0].FunctionName != "A" || hot[1].FunctionName != "B" {
		t.Fatalf("Hotspots unexpected: %#v", hot)
	}
}

func TestFormatAgentJSON_BoundedHotspotsAndOmitted(t *testing.T) {
	root := "/proj"
	metrics := []analyzer.FunctionMetrics{
		{FunctionName: "Risky", Package: "pkg", File: filepath.Join(root, "pkg/risky.go"), Line: 10, Complexity: 5, CoveragePercent: f64(20), CrapScore: f64(30.5)},
		{FunctionName: "Mild", Package: "pkg", File: filepath.Join(root, "pkg/mild.go"), Line: 4, Complexity: 3, CoveragePercent: f64(50), CrapScore: f64(9)},
		{FunctionName: "Safe", Package: "pkg", File: filepath.Join(root, "pkg/safe.go"), Line: 2, Complexity: 1, CoveragePercent: f64(100), CrapScore: f64(1)},
	}
	findings := []practices.Finding{{
		Rule:    "doccomment",
		Message: "exported func missing doc",
		Position: token.Position{
			Filename: filepath.Join(root, "pkg/risky.go"),
			Line:     10,
			Column:   1,
		},
		Description: practices.Description{
			Summary: "Document exports",
			Why:     "godoc",
			Fix:     "add a comment",
			DocURL:  "https://go.dev/doc/comment",
		},
		Confidence: 10,
	}}

	out, err := FormatAgentJSON(metrics, findings, root, 8.0, 1)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Summary struct {
			Threshold           float64 `json:"threshold"`
			MaxCrap             float64 `json:"maxCrap"`
			MaxCrapFunction     string  `json:"maxCrapFunction"`
			FunctionCount       int     `json:"functionCount"`
			AboveThresholdCount int     `json:"aboveThresholdCount"`
			FindingCount        int     `json:"findingCount"`
			HotspotLimit        int     `json:"hotspotLimit"`
		} `json:"summary"`
		Hotspots []map[string]any `json:"hotspots"`
		Findings []map[string]any `json:"findings"`
		Omitted  struct {
			FunctionsAboveHotspots int `json:"functionsAboveHotspots"`
			FunctionsTotalOmitted  int `json:"functionsTotalOmitted"`
		} `json:"omitted"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if parsed.Summary.Threshold != 8 || parsed.Summary.MaxCrap != 30.5 || parsed.Summary.MaxCrapFunction != "Risky" {
		t.Fatalf("unexpected summary: %#v", parsed.Summary)
	}
	if parsed.Summary.FunctionCount != 3 || parsed.Summary.AboveThresholdCount != 2 || parsed.Summary.FindingCount != 1 {
		t.Fatalf("unexpected counts: %#v", parsed.Summary)
	}
	// Union of above-threshold (Risky, Mild) and top-1 (Risky) → both above-threshold rows.
	if len(parsed.Hotspots) != 2 {
		t.Fatalf("expected 2 hotspots, got %#v", parsed.Hotspots)
	}
	if parsed.Hotspots[0]["functionName"] != "Risky" {
		t.Fatalf("expected Risky first: %#v", parsed.Hotspots)
	}
	if parsed.Findings[0]["why"] != "godoc" {
		t.Fatalf("missing finding why: %#v", parsed.Findings[0])
	}
	if parsed.Omitted.FunctionsTotalOmitted != 1 {
		t.Fatalf("expected 1 omitted function, got %#v", parsed.Omitted)
	}
}

func TestFormatJSON_IncludesMetricsAndFindings(t *testing.T) {
	root := "/proj"
	metrics := []analyzer.FunctionMetrics{
		{
			FunctionName:    "Risky",
			Package:         "pkg",
			File:            filepath.Join(root, "pkg/risky.go"),
			Line:            10,
			Complexity:      5,
			CoveragePercent: f64(20),
			CrapScore:       f64(30.5),
		},
	}
	findings := []practices.Finding{{
		Rule:    "doccomment",
		Message: "exported func missing doc",
		Position: token.Position{
			Filename: filepath.Join(root, "pkg/risky.go"),
			Line:     10,
			Column:   1,
		},
		Description: practices.Description{
			Summary: "Document exports",
			Why:     "godoc",
			Fix:     "add a comment",
			DocURL:  "https://go.dev/doc/comment",
		},
		Confidence: 10,
	}}

	out, err := FormatJSON(metrics, findings, root)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Functions []map[string]any `json:"functions"`
		Findings  []map[string]any `json:"findings"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(parsed.Functions) != 1 || parsed.Functions[0]["functionName"] != "Risky" {
		t.Fatalf("unexpected functions: %#v", parsed.Functions)
	}
	if parsed.Functions[0]["file"] != "pkg/risky.go" {
		t.Fatalf("expected relative file path, got %#v", parsed.Functions[0]["file"])
	}
	if len(parsed.Findings) != 1 || parsed.Findings[0]["rule"] != "doccomment" {
		t.Fatalf("unexpected findings: %#v", parsed.Findings)
	}
	if parsed.Findings[0]["why"] != "godoc" || parsed.Findings[0]["fix"] != "add a comment" {
		t.Fatalf("missing explanation fields: %#v", parsed.Findings[0])
	}
}
