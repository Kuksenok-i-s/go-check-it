// Package errorstrings flags error strings that are capitalized or end in
// punctuation. Error strings are often wrapped and printed alongside other
// context, so they should not be capitalized or end with punctuation (Go
// Wiki, "Error Strings").
package errorstrings

import (
	"go/ast"
	"strconv"
	"unicode"

	"golang.org/x/tools/go/analysis"
)

// Analyzer reports errors.New/fmt.Errorf calls with a badly styled message.
var Analyzer = &analysis.Analyzer{
	Name: "errorstrings",
	Doc:  "reports error strings that start with a capital letter or end with punctuation",
	Run:  run,
}

func run(pass *analysis.Pass) (interface{}, error) {
	for _, file := range pass.Files {
		aliases := importAliases(file, map[string]bool{"errors": true, "fmt": true})
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if ok {
				checkErrorCall(pass, call, aliases)
			}
			return true
		})
	}
	return nil, nil
}

func checkErrorCall(pass *analysis.Pass, call *ast.CallExpr, aliases map[string]string) {
	if !isErrorCtorCall(call, aliases) {
		return
	}
	msg, lit, ok := literalArg(call)
	if !ok {
		return
	}
	if issue := styleIssue(msg); issue != "" {
		pass.Reportf(lit.Pos(), "error string %s: %q", issue, msg)
	}
}

func isErrorCtorCall(call *ast.CallExpr, aliases map[string]string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkgIdent, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	pkg, ok := aliases[pkgIdent.Name]
	return ok && isErrorCtor(pkg, sel.Sel.Name)
}

func isErrorCtor(pkg, name string) bool {
	return (pkg == "errors" && name == "New") || (pkg == "fmt" && name == "Errorf")
}

func literalArg(call *ast.CallExpr) (string, *ast.BasicLit, bool) {
	if len(call.Args) == 0 {
		return "", nil, false
	}
	lit, ok := call.Args[0].(*ast.BasicLit)
	if !ok {
		return "", nil, false
	}
	msg, err := strconv.Unquote(lit.Value)
	if err != nil || msg == "" {
		return "", nil, false
	}
	return msg, lit, true
}

func styleIssue(msg string) string {
	runes := []rune(msg)
	first := runes[0]
	if unicode.IsUpper(first) && !isAllCapsWord(msg) {
		return "should not be capitalized"
	}
	last := runes[len(runes)-1]
	if last == '.' || last == '!' {
		return "should not end with punctuation"
	}
	return ""
}

// isAllCapsWord avoids flagging initialisms like "HTTP request failed".
func isAllCapsWord(msg string) bool {
	for _, r := range msg {
		if unicode.IsLower(r) {
			return false
		}
		if unicode.IsSpace(r) {
			break
		}
	}
	return true
}

// importAliases maps the local identifier a file uses for each of the
// wanted import paths (usually the package name itself, or a dot/alias).
func importAliases(file *ast.File, want map[string]bool) map[string]string {
	aliases := make(map[string]string)
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil || !want[path] {
			continue
		}
		local := path // errors, fmt: default local name equals the import path
		if imp.Name != nil {
			local = imp.Name.Name
		}
		aliases[local] = path
	}
	return aliases
}
