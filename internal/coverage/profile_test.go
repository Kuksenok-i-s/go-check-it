package coverage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseProfile_ResolvesFilesAndBlocks(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/sample\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "internal", "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}

	profile := `mode: set
example.com/sample/internal/pkg/file.go:3.10,5.2 2 1
example.com/sample/internal/pkg/file.go:6.10,8.2 1 0
`
	profilePath := filepath.Join(root, "cover.out")
	if err := os.WriteFile(profilePath, []byte(profile), 0o644); err != nil {
		t.Fatal(err)
	}

	index, err := ParseProfile(profilePath, root)
	if err != nil {
		t.Fatal(err)
	}

	wantFile := filepath.Join(root, "internal", "pkg", "file.go")
	blocks, ok := index[wantFile]
	if !ok {
		t.Fatalf("expected blocks for %s, got keys %v", wantFile, keys(index))
	}
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if blocks[0].StartLine != 3 || blocks[0].NumStmt != 2 || blocks[0].Count != 1 {
		t.Fatalf("unexpected first block: %+v", blocks[0])
	}
	if blocks[1].StartLine != 6 || blocks[1].Count != 0 {
		t.Fatalf("unexpected second block: %+v", blocks[1])
	}
}

func TestFunctionCoverage_SumsOverlappingBlocks(t *testing.T) {
	index := Index{
		"f.go": {
			{StartLine: 10, EndLine: 12, NumStmt: 3, Count: 1},
			{StartLine: 20, EndLine: 22, NumStmt: 2, Count: 0},
			{StartLine: 30, EndLine: 31, NumStmt: 1, Count: 5},
		},
	}
	total, covered := FunctionCoverage(index, "f.go", 9, 25)
	if total != 5 || covered != 3 {
		t.Fatalf("expected total=5 covered=3, got total=%d covered=%d", total, covered)
	}
}

func TestFunctionCoverage_NoBlocksIsZero(t *testing.T) {
	total, covered := FunctionCoverage(Index{}, "f.go", 1, 5)
	if total != 0 || covered != 0 {
		t.Fatalf("expected zero, got total=%d covered=%d", total, covered)
	}
}

func TestModulePath_MissingGoModIsEmpty(t *testing.T) {
	got, err := ModulePath(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("expected empty module path, got %q", got)
	}
}

func keys(index Index) []string {
	var ks []string
	for k := range index {
		ks = append(ks, k)
	}
	return ks
}
