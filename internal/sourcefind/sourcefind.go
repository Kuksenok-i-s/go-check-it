// Package sourcefind locates Go source files to analyze.
package sourcefind

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// skipDir reports whether a directory should be excluded from traversal.
func skipDir(name string) bool {
	switch name {
	case "vendor", ".git", "node_modules", "testdata":
		return true
	default:
		return strings.HasPrefix(name, ".")
	}
}

// FindAllGoFiles walks root and returns every .go file, sorted in path
// order. It skips vendor/, hidden, and testdata directories.
func FindAllGoFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}
