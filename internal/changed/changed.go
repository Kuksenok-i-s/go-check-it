// Package changed detects Go files with pending changes via git.
package changed

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// ChangedGoFiles runs `git status --porcelain` in projectRoot and returns the
// absolute paths of changed, added, or untracked .go files, sorted in path
// order. Renamed files are attributed to their new path. ctx bounds the git
// invocation, so it can be canceled (e.g. via Ctrl-C) like any other
// subprocess this tool runs.
func ChangedGoFiles(ctx context.Context, projectRoot string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", projectRoot, "status", "--porcelain")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git status failed: %s: %w", out.String(), err)
	}

	var files []string
	for _, line := range strings.Split(out.String(), "\n") {
		file := parseStatusLine(projectRoot, line)
		if file != "" {
			files = append(files, file)
		}
	}
	sort.Strings(files)
	return files, nil
}

func parseStatusLine(root, line string) string {
	if !isCandidateLine(line) {
		return ""
	}
	pathPart := strings.TrimSpace(line[3:])
	finalPath := renameTarget(pathPart)
	if !strings.HasSuffix(finalPath, ".go") {
		return ""
	}
	return filepath.Clean(filepath.Join(root, finalPath))
}

func isCandidateLine(line string) bool {
	if strings.TrimSpace(line) == "" {
		return false
	}
	return len(line) >= 4
}

func renameTarget(pathPart string) string {
	if idx := strings.Index(pathPart, " -> "); idx >= 0 {
		return pathPart[idx+4:]
	}
	return pathPart
}
