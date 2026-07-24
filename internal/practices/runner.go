package practices

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Run parses the given Go source files, groups them by directory (a
// reasonable proxy for "package" without the cost and fragility of a full
// go/packages type-checked load), and runs every registered rule over each
// group. It returns findings sorted by file, then line, then rule.
//
// Rules only see syntax, never type information: pass.TypesInfo and pass.Pkg
// are left nil. This mirrors the rest of go-check-it's file-based analysis and
// lets it run over code that doesn't fully build (e.g. mid-refactor, or
// missing a dependency), at the cost of rules like interface ownership being
// heuristic rather than soundly type-checked.
func Run(files []string) ([]Finding, error) {
	groups := groupByDir(files)
	fset := token.NewFileSet()

	var findings []Finding
	for _, group := range groups {
		groupFindings, err := runGroup(fset, group)
		if err != nil {
			return nil, err
		}
		findings = append(findings, groupFindings...)
	}

	sortFindings(findings)
	return findings, nil
}

func runGroup(fset *token.FileSet, group []string) ([]Finding, error) {
	astFiles := parseGroup(fset, group)
	if len(astFiles) == 0 {
		return nil, nil
	}

	var findings []Finding
	for _, rule := range rules {
		ruleFindings, err := runRule(fset, astFiles, rule)
		if err != nil {
			return nil, err
		}
		findings = append(findings, ruleFindings...)
	}
	return findings, nil
}

func parseGroup(fset *token.FileSet, group []string) []*ast.File {
	astFiles := make([]*ast.File, 0, len(group))
	for _, path := range group {
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			// Skip files that don't parse; other rules and other files
			// still get analyzed instead of failing the whole run.
			continue
		}
		astFiles = append(astFiles, f)
	}
	return astFiles
}

func runRule(fset *token.FileSet, astFiles []*ast.File, rule ruleEntry) ([]Finding, error) {
	var diags []analysis.Diagnostic
	pass := &analysis.Pass{
		Analyzer: rule.Analyzer,
		Fset:     fset,
		Files:    astFiles,
		Report: func(d analysis.Diagnostic) {
			diags = append(diags, d)
		},
	}
	if _, err := rule.Analyzer.Run(pass); err != nil {
		return nil, fmt.Errorf("practices: rule %s: %w", rule.Analyzer.Name, err)
	}
	findings := make([]Finding, 0, len(diags))
	for _, d := range diags {
		findings = append(findings, toFinding(rule, fset, d))
	}
	return findings, nil
}

func sortFindings(findings []Finding) {
	sort.Slice(findings, func(i, j int) bool {
		a, b := findings[i].Position, findings[j].Position
		if a.Filename != b.Filename {
			return a.Filename < b.Filename
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return findings[i].Rule < findings[j].Rule
	})
}

func toFinding(rule ruleEntry, fset *token.FileSet, d analysis.Diagnostic) Finding {
	confidence := 10
	if n, ok := parseConfidenceCategory(d.Category); ok {
		confidence = n
	}
	return Finding{
		Rule:        rule.Analyzer.Name,
		Message:     d.Message,
		Position:    fset.Position(d.Pos),
		Description: rule.Description,
		Confidence:  confidence,
	}
}

// parseConfidenceCategory extracts N from a diagnostic Category of the form
// "confidence:N", the convention heuristic rules use to self-report
// certainty (see internal/practices/goroutinelifetime).
func parseConfidenceCategory(category string) (int, bool) {
	const prefix = "confidence:"
	if !strings.HasPrefix(category, prefix) {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimPrefix(category, prefix))
	if err != nil {
		return 0, false
	}
	return n, true
}

func groupByDir(files []string) [][]string {
	byDir := make(map[string][]string)
	var dirs []string
	for _, f := range files {
		dir := filepath.Dir(f)
		if _, ok := byDir[dir]; !ok {
			dirs = append(dirs, dir)
		}
		byDir[dir] = append(byDir[dir], f)
	}
	sort.Strings(dirs)
	groups := make([][]string, 0, len(dirs))
	for _, dir := range dirs {
		groups = append(groups, byDir[dir])
	}
	return groups
}
