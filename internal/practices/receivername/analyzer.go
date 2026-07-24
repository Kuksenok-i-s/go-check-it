// Package receivername flags methods on the same type that use different
// receiver names, which reads as if they operate on different things (Go
// Wiki, "Receiver Names").
package receivername

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
)

// Analyzer reports receiver names that are inconsistent across a type's methods.
var Analyzer = &analysis.Analyzer{
	Name: "receivername",
	Doc:  "reports methods on the same type that use inconsistent receiver names",
	Run:  run,
}

type recvUse struct {
	name string
	pos  ast.Node
}

func run(pass *analysis.Pass) (interface{}, error) {
	byType, order := collectReceiverUses(pass.Files)
	reportInconsistent(pass, byType, order)
	return nil, nil
}

func collectReceiverUses(files []*ast.File) (map[string][]recvUse, []string) {
	byType := map[string][]recvUse{}
	var order []string
	for _, file := range files {
		for _, decl := range file.Decls {
			typeName, use, ok := methodReceiverUse(decl)
			if !ok {
				continue
			}
			if _, seen := byType[typeName]; !seen {
				order = append(order, typeName)
			}
			byType[typeName] = append(byType[typeName], use)
		}
	}
	return byType, order
}

func methodReceiverUse(decl ast.Decl) (string, recvUse, bool) {
	fd, ok := decl.(*ast.FuncDecl)
	if !ok || fd.Recv == nil || len(fd.Recv.List) == 0 {
		return "", recvUse{}, false
	}
	field := fd.Recv.List[0]
	if len(field.Names) == 0 || field.Names[0].Name == "_" {
		return "", recvUse{}, false
	}
	typeName := receiverTypeName(field.Type)
	if typeName == "" {
		return "", recvUse{}, false
	}
	return typeName, recvUse{name: field.Names[0].Name, pos: field}, true
}

func reportInconsistent(pass *analysis.Pass, byType map[string][]recvUse, order []string) {
	for _, typeName := range order {
		uses := byType[typeName]
		first := uses[0].name
		for _, u := range uses[1:] {
			if u.name != first {
				pass.Reportf(u.pos.Pos(), "receiver %q for %s is inconsistent with %q used elsewhere on the same type", u.name, typeName, first)
			}
		}
	}
}

func receiverTypeName(expr ast.Expr) string {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}
