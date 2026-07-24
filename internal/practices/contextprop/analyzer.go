// Package contextprop flags calls that spawn a subprocess or make an HTTP
// request without a context-aware variant, so callers cannot cancel or time
// them out (Effective Go / Go blog, "Contexts and structs").
package contextprop

import (
	"go/ast"
	"strconv"

	"golang.org/x/tools/go/analysis"
)

// Analyzer reports os/exec and net/http calls with a context-aware sibling
// that was not used.
var Analyzer = &analysis.Analyzer{
	Name: "contextprop",
	Doc:  "reports exec/http calls that ignore an available context-aware variant",
	Run:  run,
}

// httpContextReplacement is the fix suggested for net/http helpers that have
// no *Context variant of their own but can be replaced by building a
// request with context and issuing it explicitly.
const httpContextReplacement = "http.NewRequestWithContext + http.DefaultClient.Do"

// flagged maps "pkg.Func" to the context-aware replacement callers should use.
var flagged = map[string]string{
	"exec.Command":    "exec.CommandContext",
	"http.Get":        httpContextReplacement,
	"http.Post":       httpContextReplacement,
	"http.Head":       httpContextReplacement,
	"http.PostForm":   httpContextReplacement,
	"http.NewRequest": "http.NewRequestWithContext",
}

func run(pass *analysis.Pass) (interface{}, error) {
	for _, file := range pass.Files {
		aliases := importAliases(file)
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkgIdent, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			pkg, ok := aliases[pkgIdent.Name]
			if !ok {
				return true
			}
			key := pkg + "." + sel.Sel.Name
			if replacement, bad := flagged[key]; bad {
				pass.Reportf(call.Pos(), "%s ignores context cancellation; use %s instead", key, replacement)
			}
			return true
		})
	}
	return nil, nil
}

// defaultLocalName maps an import path to the package name used as its
// local identifier when the import has no explicit alias.
var defaultLocalName = map[string]string{
	"os/exec":  "exec",
	"net/http": "http",
}

// importAliases maps each local identifier a file uses for os/exec or
// net/http to that package's canonical name.
func importAliases(file *ast.File) map[string]string {
	aliases := make(map[string]string)
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		pkg, known := defaultLocalName[path]
		if !known {
			continue
		}
		local := pkg
		if imp.Name != nil {
			local = imp.Name.Name
		}
		aliases[local] = pkg
	}
	return aliases
}
