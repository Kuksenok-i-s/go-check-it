package modroot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRootFor_FindsNearestGoMod(t *testing.T) {
	root := t.TempDir()
	moduleDir := filepath.Join(root, "moda")
	if err := os.MkdirAll(filepath.Join(moduleDir, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(moduleDir, "go.mod"), []byte("module moda\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(moduleDir, "pkg", "file.go")
	if err := os.WriteFile(file, []byte("package pkg\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := RootFor(root, file)
	if got != moduleDir {
		t.Fatalf("expected %q, got %q", moduleDir, got)
	}
}

func TestRootFor_FallsBackToWorkspaceRoot(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "pkg", "file.go")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("package pkg\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := RootFor(root, file)
	if got != filepath.Clean(root) {
		t.Fatalf("expected %q, got %q", root, got)
	}
}

func TestGroupByModuleRoot_PreservesFirstSeenOrder(t *testing.T) {
	root := t.TempDir()
	modA := filepath.Join(root, "a")
	modB := filepath.Join(root, "b")
	for _, m := range []string{modA, modB} {
		if err := os.MkdirAll(m, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(m, "go.mod"), []byte("module x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	files := []string{
		filepath.Join(modB, "f1.go"),
		filepath.Join(modA, "f2.go"),
		filepath.Join(modB, "f3.go"),
	}
	order, groups := GroupByModuleRoot(root, files)
	if len(order) != 2 || order[0] != modB || order[1] != modA {
		t.Fatalf("unexpected order: %v", order)
	}
	if len(groups[modB]) != 2 || len(groups[modA]) != 1 {
		t.Fatalf("unexpected groups: %v", groups)
	}
}
