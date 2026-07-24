// Package interfaceownership flags interfaces that sit beside their own
// concrete implementation but are never referenced elsewhere in the same
// package — a sign the interface was defined by its producer instead of by
// a consumer that actually needs the abstraction (Go Wiki, "Interfaces";
// Google Go Style Guide, "Interfaces").
//
// This is a syntactic heuristic, not a type-checked analysis: it matches
// method names rather than verifying real satisfaction, and it can miss
// real local uses that happen to be masked by shadowing. It is designed to
// under-report rather than spam false positives.
package interfaceownership

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
)

// Analyzer reports producer-side interfaces with no local consumer.
var Analyzer = &analysis.Analyzer{
	Name: "interfaceownership",
	Doc:  "reports interfaces defined beside their implementation but never referenced elsewhere in the package",
	Run:  run,
}

type ifaceDecl struct {
	name    string
	pos     ast.Node
	methods map[string]bool
}

func run(pass *analysis.Pass) (interface{}, error) {
	interfaces := map[string]*ifaceDecl{}
	methodSets := map[string]map[string]bool{}
	occurrences := map[string]int{}

	for _, file := range pass.Files {
		collectTypeDecls(file, interfaces)
		collectMethodSets(file, methodSets)
	}
	for _, file := range pass.Files {
		countOccurrences(file, interfaces, occurrences)
	}

	for name, iface := range interfaces {
		if len(iface.methods) == 0 || occurrences[name] > 0 {
			continue
		}
		if impl := implementorOtherThan(name, iface.methods, methodSets); impl != "" {
			pass.Reportf(iface.pos.Pos(),
				"interface %s is defined next to its implementation %s but never used elsewhere in this package; "+
					"move it to the package that consumes it, or drop it if nothing needs the abstraction yet",
				name, impl)
		}
	}
	return nil, nil
}

func collectTypeDecls(file *ast.File, interfaces map[string]*ifaceDecl) {
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gd.Specs {
			collectInterfaceSpec(spec, interfaces)
		}
	}
}

func collectInterfaceSpec(spec ast.Spec, interfaces map[string]*ifaceDecl) {
	ts, ok := spec.(*ast.TypeSpec)
	if !ok {
		return
	}
	it, ok := ts.Type.(*ast.InterfaceType)
	if !ok {
		return
	}
	interfaces[ts.Name.Name] = &ifaceDecl{
		name:    ts.Name.Name,
		pos:     ts,
		methods: interfaceMethodNames(it),
	}
}

func interfaceMethodNames(it *ast.InterfaceType) map[string]bool {
	methods := map[string]bool{}
	for _, m := range it.Methods.List {
		for _, n := range m.Names {
			methods[n.Name] = true
		}
	}
	return methods
}

func collectMethodSets(file *ast.File, methodSets map[string]map[string]bool) {
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Recv == nil || len(fd.Recv.List) == 0 {
			continue
		}
		recvType := receiverTypeName(fd.Recv.List[0].Type)
		if recvType == "" {
			continue
		}
		if methodSets[recvType] == nil {
			methodSets[recvType] = map[string]bool{}
		}
		methodSets[recvType][fd.Name.Name] = true
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

// countOccurrences counts identifier references to any tracked interface
// name, excluding the identifier at the interface's own declaration site.
func countOccurrences(file *ast.File, interfaces map[string]*ifaceDecl, occurrences map[string]int) {
	ast.Inspect(file, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		iface, tracked := interfaces[ident.Name]
		if !tracked {
			return true
		}
		if ts, ok := iface.pos.(*ast.TypeSpec); ok && ts.Name == ident {
			return true // the declaration itself, not a use
		}
		occurrences[ident.Name]++
		return true
	})
}

func implementorOtherThan(ifaceName string, methods map[string]bool, methodSets map[string]map[string]bool) string {
	for typeName, set := range methodSets {
		if typeName == ifaceName {
			continue
		}
		if isSuperset(set, methods) {
			return typeName
		}
	}
	return ""
}

func isSuperset(set, subset map[string]bool) bool {
	for m := range subset {
		if !set[m] {
			return false
		}
	}
	return true
}
