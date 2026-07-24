// Package underscorename flags package-level identifiers that use
// underscores instead of MixedCaps (Effective Go, "MixedCaps"). Test
// function names are exempt: Test/Benchmark/Fuzz names conventionally use
// underscores to separate the target from the scenario (e.g. TestFoo_EmptyInput).
package underscorename

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Analyzer reports package-level names containing an underscore.
var Analyzer = &analysis.Analyzer{
	Name: "underscorename",
	Doc:  "reports package-level identifiers using underscores instead of MixedCaps",
	Run:  run,
}

func run(pass *analysis.Pass) (interface{}, error) {
	for _, file := range pass.Files {
		isTestFile := strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go")
		for _, decl := range file.Decls {
			checkDecl(pass, decl, isTestFile)
		}
	}
	return nil, nil
}

func checkDecl(pass *analysis.Pass, decl ast.Decl, isTestFile bool) {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		checkFuncName(pass, d, isTestFile)
	case *ast.GenDecl:
		checkGenDeclNames(pass, d)
	}
}

func checkFuncName(pass *analysis.Pass, d *ast.FuncDecl, isTestFile bool) {
	if isTestFile && isTestLikeName(d.Name.Name) {
		return
	}
	if d.Recv == nil { // only package-level funcs, not methods
		reportUnderscore(pass, d.Name)
	}
}

func checkGenDeclNames(pass *analysis.Pass, d *ast.GenDecl) {
	for _, spec := range d.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			reportUnderscore(pass, s.Name)
		case *ast.ValueSpec:
			for _, n := range s.Names {
				reportUnderscore(pass, n)
			}
		}
	}
}

func reportUnderscore(pass *analysis.Pass, id *ast.Ident) {
	if id.Name == "_" || !strings.Contains(id.Name, "_") {
		return
	}
	pass.Reportf(id.Pos(), "%s uses underscores; Go names use MixedCaps, not snake_case", id.Name)
}

func isTestLikeName(name string) bool {
	for _, prefix := range []string{"Test", "Benchmark", "Fuzz", "Example"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}
