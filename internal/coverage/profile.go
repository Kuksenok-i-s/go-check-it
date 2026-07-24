// Package coverage runs `go test -coverprofile` and parses the resulting
// coverage profile, mirroring crap4java's JacocoCoverageParser but using
// Go's native line-accurate coverage data instead of a name+line heuristic.
package coverage

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Block is a single coverage-profile statement block.
type Block struct {
	StartLine int
	EndLine   int
	NumStmt   int
	Count     int
}

// Index maps an absolute source file path to the coverage blocks recorded
// for it.
type Index map[string][]Block

// ParseProfile reads a `go test -coverprofile` file and returns an Index
// keyed by absolute file path, resolved against the module at moduleRoot.
func ParseProfile(profilePath, moduleRoot string) (Index, error) {
	f, err := os.Open(profilePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	modulePath, err := ModulePath(moduleRoot)
	if err != nil {
		return nil, err
	}
	return scanProfile(f, moduleRoot, modulePath)
}

func scanProfile(f *os.File, moduleRoot, modulePath string) (Index, error) {
	index := make(Index)
	scanner := bufio.NewScanner(f)
	first := true
	for scanner.Scan() {
		line := scanner.Text()
		if first {
			first = false
			if strings.HasPrefix(line, "mode:") {
				continue
			}
		}
		addProfileLine(index, moduleRoot, modulePath, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return index, nil
}

func addProfileLine(index Index, moduleRoot, modulePath, line string) {
	block, file, ok := parseProfileLine(line)
	if !ok {
		return
	}
	abs := resolveFile(moduleRoot, modulePath, file)
	index[abs] = append(index[abs], block)
}

// parseProfileLine parses one non-header line of a coverage profile:
//
//	<file>:<startLine>.<startCol>,<endLine>.<endCol> <numStmt> <count>
func parseProfileLine(line string) (Block, string, bool) {
	colon := strings.LastIndex(line, ":")
	if colon < 0 {
		return Block{}, "", false
	}
	file := line[:colon]
	rest := strings.Fields(line[colon+1:])
	if len(rest) != 3 {
		return Block{}, "", false
	}
	return parseProfileFields(file, rest)
}

func parseProfileFields(file string, rest []string) (Block, string, bool) {
	positions := strings.Split(rest[0], ",")
	if len(positions) != 2 {
		return Block{}, "", false
	}
	startLine, ok1 := lineOf(positions[0])
	endLine, ok2 := lineOf(positions[1])
	numStmt, err1 := strconv.Atoi(rest[1])
	count, err2 := strconv.Atoi(rest[2])
	if !ok1 || !ok2 || err1 != nil || err2 != nil {
		return Block{}, "", false
	}
	return Block{StartLine: startLine, EndLine: endLine, NumStmt: numStmt, Count: count}, file, true
}

func lineOf(posPart string) (int, bool) {
	dot := strings.Index(posPart, ".")
	if dot < 0 {
		return 0, false
	}
	line, err := strconv.Atoi(posPart[:dot])
	if err != nil {
		return 0, false
	}
	return line, true
}

func resolveFile(moduleRoot, modulePath, profileFile string) string {
	if modulePath != "" {
		if rel, ok := cutPrefix(profileFile, modulePath); ok {
			rel = strings.TrimPrefix(rel, "/")
			return filepath.Join(moduleRoot, filepath.FromSlash(rel))
		}
	}
	if filepath.IsAbs(profileFile) {
		return filepath.Clean(profileFile)
	}
	return filepath.Join(moduleRoot, filepath.FromSlash(profileFile))
}

func cutPrefix(s, prefix string) (string, bool) {
	if !strings.HasPrefix(s, prefix) {
		return s, false
	}
	return s[len(prefix):], true
}

// ModulePath reads the `module` directive from go.mod at moduleRoot. It
// returns "" (no error) if go.mod does not exist.
func ModulePath(moduleRoot string) (string, error) {
	data, err := os.ReadFile(filepath.Join(moduleRoot, "go.mod"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module")), nil
		}
	}
	return "", fmt.Errorf("no module directive found in %s", filepath.Join(moduleRoot, "go.mod"))
}

// FunctionCoverage sums the statement counts recorded for a function
// spanning [startLine, endLine] in file. It returns (0, 0) if no coverage
// blocks fall within the function, meaning coverage is unknown.
func FunctionCoverage(index Index, file string, startLine, endLine int) (total, covered int) {
	for _, block := range index[file] {
		if block.StartLine < startLine || block.StartLine > endLine {
			continue
		}
		total += block.NumStmt
		if block.Count > 0 {
			covered += block.NumStmt
		}
	}
	return total, covered
}
