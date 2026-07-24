package cliapp

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
)

// Flag names and format values, defined once and reused across parsing,
// help text, and validation.
const (
	flagHelp           = "--help"
	flagChanged        = "--changed"
	flagExplain        = "--explain"
	flagFailOnFindings = "--fail-on-findings"
	flagMaxWorkers     = "--max-workers"
	flagFormatPrefix   = "--format="

	formatText = "text"
	formatJSON = "json"
)

// Mode selects which set of files go-check-it analyzes.
type Mode int

const (
	// ModeAllSrc analyzes every Go source file found in the project.
	ModeAllSrc Mode = iota
	// ModeChangedSrc analyzes only Go files reported as changed by git.
	ModeChangedSrc
	// ModeExplicitFiles analyzes only the files/directories given on the CLI.
	ModeExplicitFiles
	// ModeHelp prints usage and exits.
	ModeHelp
)

// Arguments is the parsed form of the CLI invocation.
type Arguments struct {
	Mode     Mode
	FileArgs []string

	// Explain prints each practice finding's full rationale, fix, and doc
	// link instead of a one-line message.
	Explain bool
	// FailOnFindings makes any practice finding return a non-zero exit code.
	FailOnFindings bool
	// Format is "text" (default) or "json".
	Format string
	// MaxWorkers is the maximum number of modules analyzed concurrently.
	// Defaults to half the number of logical CPUs (at least 1).
	MaxWorkers int
}

// ParseArgs parses raw CLI arguments into Arguments.
//
// Rules mirror crap4java's CliArgumentsParser:
//   - no args                     -> ModeAllSrc
//   - "--help" anywhere           -> ModeHelp
//   - "--changed" with no other positional args -> ModeChangedSrc
//   - "--changed" combined with positional args -> error
//   - otherwise, non-flag args become the explicit file list
//
// Additionally, "--explain", "--fail-on-findings", "--format=text|json",
// and "--max-workers N" are recognized in any mode.
func ParseArgs(args []string) (Arguments, error) {
	if containsFlag(args, flagHelp) {
		return Arguments{Mode: ModeHelp, MaxWorkers: defaultMaxWorkers()}, nil
	}
	if len(args) == 0 {
		return Arguments{Mode: ModeAllSrc, Format: formatText, MaxWorkers: defaultMaxWorkers()}, nil
	}
	return parseArgsFlags(args)
}

func parseArgsFlags(args []string) (Arguments, error) {
	maxWorkers, rest, err := extractMaxWorkers(args)
	if err != nil {
		return Arguments{}, err
	}
	format, err := parseFormat(rest)
	if err != nil {
		return Arguments{}, err
	}
	changed := containsFlag(rest, flagChanged)
	values := nonFlagArgs(rest)
	if changed && len(values) > 0 {
		return Arguments{}, fmt.Errorf("--changed cannot be combined with file arguments")
	}
	return Arguments{
		Mode:           selectMode(changed, values, rest),
		FileArgs:       values,
		Explain:        containsFlag(rest, flagExplain),
		FailOnFindings: containsFlag(rest, flagFailOnFindings),
		Format:         format,
		MaxWorkers:     maxWorkers,
	}, nil
}

func selectMode(changed bool, values, args []string) Mode {
	// An unrecognized flag (likely a typo) falls back to ModeExplicitFiles
	// with no files rather than silently analyzing the whole project.
	switch {
	case changed:
		return ModeChangedSrc
	case len(values) > 0 || hasUnknownFlag(args):
		return ModeExplicitFiles
	default:
		return ModeAllSrc
	}
}

var knownFlags = map[string]bool{
	flagHelp:           true,
	flagChanged:        true,
	flagExplain:        true,
	flagFailOnFindings: true,
}

func hasUnknownFlag(args []string) bool {
	for _, arg := range args {
		if !strings.HasPrefix(arg, "--") {
			continue
		}
		if strings.HasPrefix(arg, flagFormatPrefix) {
			continue
		}
		if !knownFlags[arg] {
			return true
		}
	}
	return false
}

func parseFormat(args []string) (string, error) {
	format := formatText
	for _, arg := range args {
		if !strings.HasPrefix(arg, flagFormatPrefix) {
			continue
		}
		format = strings.TrimPrefix(arg, flagFormatPrefix)
	}
	if format != formatText && format != formatJSON {
		return "", fmt.Errorf("--format must be %q or %q, got %q", formatText, formatJSON, format)
	}
	return format, nil
}

// extractMaxWorkers pulls --max-workers N out of args and returns the worker
// count plus the remaining args. When the flag is absent, the default is used.
func extractMaxWorkers(args []string) (int, []string, error) {
	workers := defaultMaxWorkers()
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] != flagMaxWorkers {
			out = append(out, args[i])
			continue
		}
		i++
		if i >= len(args) {
			return 0, nil, fmt.Errorf("--max-workers requires a positive integer")
		}
		n, err := strconv.Atoi(args[i])
		if err != nil || n < 1 {
			return 0, nil, fmt.Errorf("--max-workers requires a positive integer")
		}
		workers = n
	}
	return workers, out, nil
}

func defaultMaxWorkers() int {
	workers := runtime.NumCPU() / 2
	if workers < 1 {
		return 1
	}
	return workers
}

func containsFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}

func nonFlagArgs(args []string) []string {
	values := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.HasPrefix(arg, "--") {
			continue
		}
		values = append(values, arg)
	}
	return values
}

// Usage returns the CLI help text.
func Usage() string {
	return `Usage:
  go-check-it                    Analyze all Go files in the project
  go-check-it --changed          Analyze changed Go files (git status --porcelain)
  go-check-it <path...>          Analyze files, or for directory args analyze <dir>/**/*.go
  go-check-it --help             Print this help message

Flags (combine with any mode above):
  --explain                  Print each practice finding's rationale, fix, and doc link
  --fail-on-findings         Exit non-zero if any practice finding is reported
  --format=text|json         Output format (default text)
  --max-workers N            Analyze up to N modules in parallel (default: half the CPUs)
`
}
