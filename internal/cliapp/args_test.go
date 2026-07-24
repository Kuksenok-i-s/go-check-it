package cliapp

import (
	"runtime"
	"testing"
)

func TestParseArgs_NoArgsIsAllSrc(t *testing.T) {
	got, err := ParseArgs(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Mode != ModeAllSrc {
		t.Fatalf("expected ModeAllSrc, got %v", got.Mode)
	}
	wantWorkers := runtime.NumCPU() / 2
	if wantWorkers < 1 {
		wantWorkers = 1
	}
	if got.MaxWorkers != wantWorkers {
		t.Fatalf("expected default MaxWorkers %d, got %d", wantWorkers, got.MaxWorkers)
	}
}

func TestParseArgs_Help(t *testing.T) {
	got, err := ParseArgs([]string{"--help"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Mode != ModeHelp {
		t.Fatalf("expected ModeHelp, got %v", got.Mode)
	}
}

func TestParseArgs_Changed(t *testing.T) {
	got, err := ParseArgs([]string{"--changed"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Mode != ModeChangedSrc {
		t.Fatalf("expected ModeChangedSrc, got %v", got.Mode)
	}
}

func TestParseArgs_ChangedCombinedWithFilesErrors(t *testing.T) {
	_, err := ParseArgs([]string{"--changed", "foo.go"})
	if err == nil {
		t.Fatal("expected error combining --changed with file args")
	}
}

func TestParseArgs_ExplicitFiles(t *testing.T) {
	got, err := ParseArgs([]string{"a.go", "b.go"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Mode != ModeExplicitFiles {
		t.Fatalf("expected ModeExplicitFiles, got %v", got.Mode)
	}
	if len(got.FileArgs) != 2 || got.FileArgs[0] != "a.go" || got.FileArgs[1] != "b.go" {
		t.Fatalf("unexpected file args: %v", got.FileArgs)
	}
}

func TestParseArgs_UnknownFlagIsIgnoredAsValue(t *testing.T) {
	got, err := ParseArgs([]string{"--unknown"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Mode != ModeExplicitFiles || len(got.FileArgs) != 0 {
		t.Fatalf("expected ModeExplicitFiles with no file args, got %+v", got)
	}
}

func TestParseArgs_KnownFlagsAloneStayAllSrc(t *testing.T) {
	got, err := ParseArgs([]string{"--explain", "--fail-on-findings", "--format=json"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Mode != ModeAllSrc {
		t.Fatalf("expected ModeAllSrc, got %v", got.Mode)
	}
	if !got.Explain || !got.FailOnFindings || got.Format != "json" {
		t.Fatalf("flags not parsed correctly: %+v", got)
	}
}

func TestParseArgs_DefaultFormatIsText(t *testing.T) {
	got, err := ParseArgs([]string{"a.go"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Format != "text" {
		t.Fatalf("expected default format text, got %q", got.Format)
	}
}

func TestParseArgs_InvalidFormatErrors(t *testing.T) {
	_, err := ParseArgs([]string{"--format=yaml"})
	if err == nil {
		t.Fatal("expected error for invalid --format value")
	}
}

func TestParseArgs_MaxWorkersExplicit(t *testing.T) {
	got, err := ParseArgs([]string{"--max-workers", "4", "--explain"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.MaxWorkers != 4 {
		t.Fatalf("expected MaxWorkers 4, got %d", got.MaxWorkers)
	}
	if got.Mode != ModeAllSrc || !got.Explain {
		t.Fatalf("unexpected args: %+v", got)
	}
}

func TestParseArgs_MaxWorkersWithFiles(t *testing.T) {
	got, err := ParseArgs([]string{"--max-workers", "2", "a.go"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.MaxWorkers != 2 {
		t.Fatalf("expected MaxWorkers 2, got %d", got.MaxWorkers)
	}
	if len(got.FileArgs) != 1 || got.FileArgs[0] != "a.go" {
		t.Fatalf("expected file args [a.go], got %v", got.FileArgs)
	}
}

func TestParseArgs_MaxWorkersMissingValue(t *testing.T) {
	_, err := ParseArgs([]string{"--max-workers"})
	if err == nil {
		t.Fatal("expected error for missing --max-workers value")
	}
}

func TestParseArgs_MaxWorkersInvalidValue(t *testing.T) {
	for _, args := range [][]string{
		{"--max-workers", "0"},
		{"--max-workers", "-1"},
		{"--max-workers", "abc"},
	} {
		_, err := ParseArgs(args)
		if err == nil {
			t.Fatalf("expected error for %v", args)
		}
	}
}
