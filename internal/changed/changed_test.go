package changed

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestIsCandidateLine(t *testing.T) {
	cases := map[string]bool{
		"":          false,
		"   ":       false,
		"M ":        false,
		" M a.go":   true,
		"?? new.go": true,
	}
	for line, want := range cases {
		if got := isCandidateLine(line); got != want {
			t.Errorf("isCandidateLine(%q) = %v, want %v", line, got, want)
		}
	}
}

func TestRenameTarget(t *testing.T) {
	if got := renameTarget("old.go -> new.go"); got != "new.go" {
		t.Fatalf("expected new.go, got %q", got)
	}
	if got := renameTarget("plain.go"); got != "plain.go" {
		t.Fatalf("expected plain.go, got %q", got)
	}
}

func TestParseStatusLine(t *testing.T) {
	root := "/proj"
	if got := parseStatusLine(root, " M internal/foo.go"); got != "/proj/internal/foo.go" {
		t.Fatalf("got %q", got)
	}
	if got := parseStatusLine(root, "?? README.md"); got != "" {
		t.Fatalf("expected empty for non-go file, got %q", got)
	}
	if got := parseStatusLine(root, "R  old.go -> new.go"); got != "/proj/new.go" {
		t.Fatalf("got %q", got)
	}
	if got := parseStatusLine(root, ""); got != "" {
		t.Fatalf("expected empty for blank line, got %q", got)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func TestChangedGoFiles_ReportsUntrackedAndModified(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init", "-q")
	committed := filepath.Join(root, "committed.go")
	if err := os.WriteFile(committed, []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-q", "-m", "init")

	// Modify the committed file and add a new untracked one.
	if err := os.WriteFile(committed, []byte("package p\n\nfunc A() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	untracked := filepath.Join(root, "new.go")
	if err := os.WriteFile(untracked, []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ChangedGoFiles(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{committed: true, untracked: true}
	if len(got) != 2 {
		t.Fatalf("expected 2 changed .go files, got %v", got)
	}
	for _, f := range got {
		if !want[f] {
			t.Fatalf("unexpected file in changed set: %s (all: %v)", f, got)
		}
	}
}

func TestChangedGoFiles_NonGitDirectoryErrors(t *testing.T) {
	_, err := ChangedGoFiles(context.Background(), t.TempDir())
	if err == nil {
		t.Fatal("expected error for non-git directory")
	}
}

func TestChangedGoFiles_ContextCanceledReturnsError(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init", "-q")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ChangedGoFiles(ctx, root)
	if err == nil {
		t.Fatal("expected an error for a canceled context")
	}
}
