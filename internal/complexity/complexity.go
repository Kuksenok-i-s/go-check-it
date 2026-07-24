// Package complexity parses Go source files and computes per-function
// cyclomatic complexity, mirroring crap4java's JavaMethodParser.
package complexity

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
)

// FuncDescriptor describes a single top-level function or method.
type FuncDescriptor struct {
	Name       string // e.g. "NewServer" or "(*Server).Handle"
	Package    string
	File       string
	StartLine  int
	EndLine    int
	Complexity int
}

// ParseFile parses a single Go source file and returns a descriptor for
// every concrete function/method declaration in it.
//
// Interface method declarations (no body) and function literals are not
// reported as standalone units: interface methods have no body to analyze,
// and function literals are anonymous, so any control flow inside them is
// folded into the complexity of their enclosing declaration.
func ParseFile(path string) ([]FuncDescriptor, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	pkg := ""
	if file.Name != nil {
		pkg = file.Name.Name
	}

	var descriptors []FuncDescriptor
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		descriptors = append(descriptors, FuncDescriptor{
			Name:       funcDisplayName(fn),
			Package:    pkg,
			File:       path,
			StartLine:  fset.Position(fn.Pos()).Line,
			EndLine:    fset.Position(fn.End()).Line,
			Complexity: complexityOf(fn.Body),
		})
	}
	return descriptors, nil
}

func funcDisplayName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	recvType := types.ExprString(fn.Recv.List[0].Type)
	return "(" + recvType + ")." + fn.Name.Name
}

// complexityOf computes cyclomatic complexity for a function body.
//
// Complexity starts at 1 and is incremented for each decision point:
// if, for/range, switch/type-switch/select cases, and short-circuit
// logical operators (&&, ||). It descends into nested function literals so
// that closures contribute to the complexity of their enclosing function.
func complexityOf(body *ast.BlockStmt) int {
	complexity := 1
	ast.Inspect(body, func(n ast.Node) bool {
		complexity += decisionPoints(n)
		return true
	})
	return complexity
}

func decisionPoints(n ast.Node) int {
	switch node := n.(type) {
	case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.CaseClause, *ast.CommClause:
		return 1
	case *ast.BinaryExpr:
		if node.Op == token.LAND || node.Op == token.LOR {
			return 1
		}
	}
	return 0
}
