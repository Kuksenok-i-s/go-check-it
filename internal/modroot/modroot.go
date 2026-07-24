// Package modroot determines the owning Go module for a source file.
package modroot

import (
	"os"
	"path/filepath"
	"strings"
)

// RootFor returns the nearest ancestor directory of file that contains a
// go.mod, without walking above workspaceRoot. If no go.mod is found,
// workspaceRoot is returned, mirroring crap4java's Maven pom.xml lookup.
func RootFor(workspaceRoot, file string) string {
	root := filepath.Clean(workspaceRoot)
	current := filepath.Clean(file)
	if info, err := os.Stat(current); err != nil || !info.IsDir() {
		current = filepath.Dir(current)
	}

	for current != "" && withinRoot(root, current) {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return root
}

func withinRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || !strings.HasPrefix(rel, "..")
}

// GroupByModuleRoot groups files by their owning module root, preserving the
// order in which module roots are first encountered.
func GroupByModuleRoot(workspaceRoot string, files []string) (order []string, groups map[string][]string) {
	groups = make(map[string][]string)
	for _, file := range files {
		root := RootFor(workspaceRoot, file)
		if _, ok := groups[root]; !ok {
			order = append(order, root)
		}
		groups[root] = append(groups[root], file)
	}
	return order, groups
}
