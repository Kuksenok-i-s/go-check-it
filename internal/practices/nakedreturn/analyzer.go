// Package nakedreturn flags naked returns in functions long enough that the
// returned values are no longer obvious to a reader (Go Wiki, "Named Result
// Parameters").
package nakedreturn

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
)

// MaxLines is the body length (in source lines) above which a naked return
// is considered to hurt readability.
const MaxLines = 20

// Analyzer reports naked returns in functions longer than MaxLines lines.
var Analyzer = &analysis.Analyzer{
	Name: "nakedreturn",
	Doc:  "reports naked returns in functions longer than a threshold",
	Run:  run,
}

func run(pass *analysis.Pass) (interface{}, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			fd, ok := n.(*ast.FuncDecl)
			if ok {
				checkFunc(pass, fd)
			}
			return true
		})
	}
	return nil, nil
}

func checkFunc(pass *analysis.Pass, fd *ast.FuncDecl) {
	if fd.Body == nil || fd.Type.Results == nil {
		return
	}
	if !hasNamedResults(fd.Type.Results) {
		return
	}
	if lineCount(pass, fd.Body) <= MaxLines {
		return
	}
	reportNakedReturns(pass, fd)
}

func reportNakedReturns(pass *analysis.Pass, fd *ast.FuncDecl) {
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		ret, ok := n.(*ast.ReturnStmt)
		if ok && len(ret.Results) == 0 {
			pass.Reportf(ret.Pos(), "naked return in %s (%d+ line body); name the returned values explicitly", fd.Name.Name, MaxLines)
		}
		return true
	})
}

func hasNamedResults(fl *ast.FieldList) bool {
	for _, f := range fl.List {
		if len(f.Names) > 0 {
			return true
		}
	}
	return false
}

func lineCount(pass *analysis.Pass, body *ast.BlockStmt) int {
	start := pass.Fset.Position(body.Pos()).Line
	end := pass.Fset.Position(body.End()).Line
	return end - start
}
