package complexity

import (
	"os"
	"path/filepath"
	"testing"
)

func parseSource(t *testing.T, src string) []FuncDescriptor {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.go")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func findFunc(t *testing.T, funcs []FuncDescriptor, name string) FuncDescriptor {
	t.Helper()
	for _, f := range funcs {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("function %q not found in %+v", name, funcs)
	return FuncDescriptor{}
}

func TestParseFile_SimpleFunctionComplexityOne(t *testing.T) {
	src := `package p

func Simple() int {
	return 1
}
`
	funcs := parseSource(t, src)
	f := findFunc(t, funcs, "Simple")
	if f.Complexity != 1 {
		t.Fatalf("expected complexity 1, got %d", f.Complexity)
	}
	if f.Package != "p" {
		t.Fatalf("expected package p, got %s", f.Package)
	}
}

func TestParseFile_IfAndLogicalOperators(t *testing.T) {
	src := `package p

func Branchy(a, b int) int {
	if a > 0 && b > 0 {
		return a
	}
	if a < 0 || b < 0 {
		return b
	}
	return 0
}
`
	f := findFunc(t, parseSource(t, src), "Branchy")
	// base 1 + if + && + if + || = 5
	if f.Complexity != 5 {
		t.Fatalf("expected complexity 5, got %d", f.Complexity)
	}
}

func TestParseFile_LoopsAndSwitch(t *testing.T) {
	src := `package p

func Loopy(items []int) int {
	total := 0
	for _, item := range items {
		switch {
		case item > 0:
			total += item
		case item < 0:
			total -= item
		default:
			total += 0
		}
	}
	for i := 0; i < 10; i++ {
		total++
	}
	return total
}
`
	f := findFunc(t, parseSource(t, src), "Loopy")
	// base 1 + range + case + case + default + for = 6
	if f.Complexity != 6 {
		t.Fatalf("expected complexity 6, got %d", f.Complexity)
	}
}

func TestParseFile_MethodWithPointerReceiver(t *testing.T) {
	src := `package p

type Server struct{}

func (s *Server) Handle() {
	if s != nil {
	}
}
`
	funcs := parseSource(t, src)
	f := findFunc(t, funcs, "(*Server).Handle")
	if f.Complexity != 2 {
		t.Fatalf("expected complexity 2, got %d", f.Complexity)
	}
}

func TestParseFile_ClosureComplexityFoldsIntoEnclosing(t *testing.T) {
	src := `package p

func WithClosure() int {
	fn := func(x int) int {
		if x > 0 {
			return x
		}
		return -x
	}
	return fn(1)
}
`
	funcs := parseSource(t, src)
	if len(funcs) != 1 {
		t.Fatalf("expected only the enclosing function to be reported, got %+v", funcs)
	}
	f := findFunc(t, funcs, "WithClosure")
	if f.Complexity != 2 {
		t.Fatalf("expected complexity 2 (closure if folded in), got %d", f.Complexity)
	}
}

func TestParseFile_InterfaceMethodsAreNotReported(t *testing.T) {
	src := `package p

type Doer interface {
	Do() error
}

func Concrete() {}
`
	funcs := parseSource(t, src)
	if len(funcs) != 1 || funcs[0].Name != "Concrete" {
		t.Fatalf("expected only Concrete to be reported, got %+v", funcs)
	}
}
