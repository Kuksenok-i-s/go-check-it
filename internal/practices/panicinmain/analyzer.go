// Package panicinmain flags calls to panic within package main. A command's
// main package should report errors and exit with a non-zero status instead
// of crashing with a stack trace (Effective Go, "Errors").
package panicinmain

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
)

// Analyzer reports panic() calls reachable from package main.
var Analyzer = &analysis.Analyzer{
	Name: "panicinmain",
	Doc:  "reports panic() calls in package main; main should log and os.Exit instead of crashing",
	Run:  run,
}

func run(pass *analysis.Pass) (interface{}, error) {
	for _, file := range pass.Files {
		if file.Name == nil || file.Name.Name != "main" {
			continue
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			ident, ok := call.Fun.(*ast.Ident)
			if !ok || ident.Name != "panic" {
				return true
			}
			pass.Reportf(call.Pos(), "panic() in package main; return an error and let main log it and os.Exit(1) instead")
			return true
		})
	}
	return nil, nil
}
