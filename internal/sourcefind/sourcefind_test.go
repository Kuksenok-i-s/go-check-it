package sourcefind

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFindAllGoFiles_FindsAndSorts(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "b.go"), "package p\n")
	writeFile(t, filepath.Join(root, "a.go"), "package p\n")
	writeFile(t, filepath.Join(root, "notgo.txt"), "x")
	writeFile(t, filepath.Join(root, "sub", "c.go"), "package p\n")

	got, err := FindAllGoFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		filepath.Join(root, "a.go"),
		filepath.Join(root, "b.go"),
		filepath.Join(root, "sub", "c.go"),
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestFindAllGoFiles_SkipsVendorAndHidden(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "vendor", "dep.go"), "package p\n")
	writeFile(t, filepath.Join(root, ".hidden", "h.go"), "package p\n")
	writeFile(t, filepath.Join(root, "keep.go"), "package p\n")

	got, err := FindAllGoFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != filepath.Join(root, "keep.go") {
		t.Fatalf("expected only keep.go, got %v", got)
	}
}

func TestFindAllGoFiles_MissingRootIsError(t *testing.T) {
	_, err := FindAllGoFiles(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("expected error for missing root")
	}
}
