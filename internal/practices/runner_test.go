package practices_test

import (
	"os"
	"path/filepath"
	"testing"

	"go-check-it/internal/practices"
)

func TestRun_FindsAcrossRules(t *testing.T) {
	dir := t.TempDir()
	src := `package main

import "errors"

var errBad = errors.New("Bad message.")

func main() {
	panic("boom")
}
`
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	findings, err := practices.Run([]string{path})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	got := map[string]bool{}
	for _, f := range findings {
		got[f.Rule] = true
		if f.Description.Summary == "" {
			t.Errorf("finding for rule %s has no description", f.Rule)
		}
		if f.Position.Filename == "" {
			t.Errorf("finding for rule %s has no position", f.Rule)
		}
	}
	for _, want := range []string{"panicinmain", "errorstrings"} {
		if !got[want] {
			t.Errorf("expected a finding from rule %s, got rules: %v", want, got)
		}
	}
}

func TestRun_NoFindingsOnCleanCode(t *testing.T) {
	dir := t.TempDir()
	src := `package p

// Add returns the sum of a and b.
func Add(a, b int) int {
	return a + b
}
`
	path := filepath.Join(dir, "p.go")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	findings, err := practices.Run([]string{path})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected no findings, got %+v", findings)
	}
}
