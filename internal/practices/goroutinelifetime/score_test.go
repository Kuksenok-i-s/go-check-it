package goroutinelifetime

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// bodyOf parses a single go-statement wrapped in a throwaway function and
// returns the launched closure's body, for directly exercising scoreBody.
func bodyOf(t *testing.T, goStmtSrc string) *ast.BlockStmt {
	t.Helper()
	src := "package p\nfunc _() {\n" + goStmtSrc + "\n}\n"
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "p.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var body *ast.BlockStmt
	ast.Inspect(file, func(n ast.Node) bool {
		if g, ok := n.(*ast.GoStmt); ok {
			if lit, ok := g.Call.Fun.(*ast.FuncLit); ok {
				body = lit.Body
			}
		}
		return true
	})
	if body == nil {
		t.Fatalf("no goroutine literal found in: %s", src)
	}
	return body
}

func TestScoreBody(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantMin int
		wantMax int
	}{
		{
			name:    "bare infinite loop is high risk",
			src:     `go func() { for { work() } }()`,
			wantMin: 9,
			wantMax: 10,
		},
		{
			name:    "select on ctx.Done is low risk",
			src:     `go func() { for { select { case <-ctx.Done(): return; default: work() } } }()`,
			wantMin: 0,
			wantMax: 0,
		},
		{
			name:    "waitgroup Done call reduces risk",
			src:     `go func() { defer wg.Done(); work() }()`,
			wantMin: 0,
			wantMax: 2,
		},
		{
			name:    "plain call with a return is neutral-low",
			src:     `go func() { if err := work(); err != nil { return } }()`,
			wantMin: 3,
			wantMax: 5,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scoreBody(bodyOf(t, tt.src))
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("scoreBody(%q) = %d, want in [%d,%d]", tt.src, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}
