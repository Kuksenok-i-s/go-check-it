// Package doccomment flags doc comments on exported declarations that don't
// follow the "Name means/does ..." convention or don't end in a period
// (Effective Go, "Commentary"; Go Wiki, "Comment Sentences").
package doccomment

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Analyzer reports exported declarations whose doc comment doesn't start
// with the declared name or doesn't end with a period.
var Analyzer = &analysis.Analyzer{
	Name: "doccomment",
	Doc:  "reports exported decl doc comments that don't start with the name or end with a period",
	Run:  run,
}

func run(pass *analysis.Pass) (interface{}, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			checkDecl(pass, decl)
		}
	}
	return nil, nil
}

func checkDecl(pass *analysis.Pass, decl ast.Decl) {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		check(pass, d.Doc, d.Name)
	case *ast.GenDecl:
		checkGenDecl(pass, d)
	}
}

func checkGenDecl(pass *analysis.Pass, d *ast.GenDecl) {
	// A parenthesized block ("const (...)", "var (...)") with a single
	// shared Doc is conventionally a group comment (e.g. "// Exit codes are
	// ..."), not a per-name doc, so it isn't held to the "starts with the
	// name" rule — only individual specs with their own Doc are.
	groupDoc := d.Lparen.IsValid() && len(d.Specs) > 1
	for i, spec := range d.Specs {
		name, doc := specNameDoc(spec)
		if name == nil {
			continue
		}
		if doc == nil && i == 0 && !groupDoc {
			doc = d.Doc
		}
		check(pass, doc, name)
	}
}

func specNameDoc(spec ast.Spec) (*ast.Ident, *ast.CommentGroup) {
	switch s := spec.(type) {
	case *ast.TypeSpec:
		return s.Name, s.Doc
	case *ast.ValueSpec:
		if len(s.Names) == 0 {
			return nil, nil
		}
		return s.Names[0], s.Doc
	}
	return nil, nil
}

func check(pass *analysis.Pass, doc *ast.CommentGroup, name *ast.Ident) {
	if doc == nil || name == nil || !name.IsExported() {
		return
	}
	text := strings.TrimSpace(doc.Text())
	if text == "" {
		return
	}
	reportDocIssues(pass, name, text)
}

func reportDocIssues(pass *analysis.Pass, name *ast.Ident, text string) {
	if !strings.HasPrefix(text, name.Name+" ") && text != name.Name {
		pass.Reportf(name.Pos(), "doc comment for %s should start with %q", name.Name, name.Name)
		return
	}
	if !isSentenceEnd(lastRune(text)) {
		pass.Reportf(name.Pos(), "doc comment for %s should end with a period", name.Name)
	}
}

func lastRune(s string) rune {
	r := []rune(strings.TrimRight(s, " \t\n"))
	if len(r) == 0 {
		return 0
	}
	return r[len(r)-1]
}

func isSentenceEnd(r rune) bool {
	return r == '.' || r == '!' || r == '?'
}
