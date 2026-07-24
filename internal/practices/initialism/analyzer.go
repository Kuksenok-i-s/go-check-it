// Package initialism flags exported identifiers that use inconsistent
// casing for common initialisms, e.g. Id, Url, Http instead of ID, URL,
// HTTP (Go Wiki, "Initialisms").
package initialism

import (
	"go/ast"
	"regexp"

	"golang.org/x/tools/go/analysis"
)

// Analyzer reports exported identifiers with wrongly cased initialisms.
var Analyzer = &analysis.Analyzer{
	Name: "initialism",
	Doc:  "reports exported identifiers using wrongly cased initialisms (Id, Url, Http, ...)",
	Run:  run,
}

// common lists initialisms Go convention keeps fully uppercase (or fully
// lowercase mid-word), keyed by their common mis-cased form.
var common = map[string]string{
	"Id":    "ID",
	"Ids":   "IDs",
	"Url":   "URL",
	"Urls":  "URLs",
	"Http":  "HTTP",
	"Https": "HTTPS",
	"Api":   "API",
	"Json":  "JSON",
	"Xml":   "XML",
	"Html":  "HTML",
	"Uuid":  "UUID",
	"Uri":   "URI",
	"Sql":   "SQL",
}

var wordRe = regexp.MustCompile(`[A-Z][a-z]*`)

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
		reportInitialism(pass, d.Name)
	case *ast.GenDecl:
		checkGenDecl(pass, d)
	}
}

func checkGenDecl(pass *analysis.Pass, d *ast.GenDecl) {
	for _, spec := range d.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			reportInitialism(pass, s.Name)
		case *ast.ValueSpec:
			for _, n := range s.Names {
				reportInitialism(pass, n)
			}
		}
	}
}

func reportInitialism(pass *analysis.Pass, id *ast.Ident) {
	if !id.IsExported() {
		return
	}
	if bad := findBadInitialism(id.Name); bad != "" {
		pass.Reportf(id.Pos(), "%s uses %q; Go convention keeps initialisms like this fully uppercase (%s)", id.Name, bad, common[bad])
	}
}

// findBadInitialism returns the first mis-cased initialism word found as a
// standalone component of a MixedCaps identifier, or "" if none.
func findBadInitialism(name string) string {
	for _, word := range wordRe.FindAllString(name, -1) {
		if _, found := common[word]; found {
			return word
		}
	}
	return ""
}
