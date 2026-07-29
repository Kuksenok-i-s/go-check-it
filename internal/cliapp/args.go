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
	flagHelp            = "--help"
	flagChanged         = "--changed"
	flagExplain         = "--explain"
	flagFailOnFindings  = "--fail-on-findings"
	flagMaxWorkers      = "--max-workers"
	flagTopPrefix       = "--top="
	flagFormatPrefix    = "--format="
	flagThresholdPrefix = "--threshold="

	formatText      = "text"
	formatJSON      = "json"
	formatAgentJSON = "agent-json"

	// DefaultTopN is the default hotspot limit for --format=agent-json.
	DefaultTopN = 6
)

// DefaultThreshold is the CRAP score above which a run fails when
// --threshold is not given.
const DefaultThreshold = 8.0

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
	// Format is "text" (default), "json", or "agent-json".
	Format string
	// MaxWorkers is the maximum number of modules analyzed concurrently.
	// Defaults to half the number of logical CPUs (at least 1).
	MaxWorkers int
	// Threshold is the CRAP score above which the run fails. Defaults to
	// DefaultThreshold.
	Threshold float64
	// TopN limits CRAP hotspots in --format=agent-json. Defaults to
	// DefaultTopN. Ignored for text/json formats.
	TopN int
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
// "--max-workers N", and "--threshold=N" are recognized in any mode.
func ParseArgs(args []string) (Arguments, error) {
	if containsFlag(args, flagHelp) {
		return Arguments{
			Mode:       ModeHelp,
			MaxWorkers: defaultMaxWorkers(),
			Threshold:  DefaultThreshold,
			TopN:       DefaultTopN,
		}, nil
	}
	if len(args) == 0 {
		return Arguments{
			Mode:       ModeAllSrc,
			Format:     formatText,
			MaxWorkers: defaultMaxWorkers(),
			Threshold:  DefaultThreshold,
			TopN:       DefaultTopN,
		}, nil
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
	threshold, err := parseThreshold(rest)
	if err != nil {
		return Arguments{}, err
	}
	topN, err := parseTopN(rest)
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
		Threshold:      threshold,
		TopN:           topN,
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
		if strings.HasPrefix(arg, flagThresholdPrefix) {
			continue
		}
		if strings.HasPrefix(arg, flagTopPrefix) {
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
	switch format {
	case formatText, formatJSON, formatAgentJSON:
		return format, nil
	default:
		return "", fmt.Errorf("--format must be %q, %q, or %q, got %q", formatText, formatJSON, formatAgentJSON, format)
	}
}

// parseTopN pulls --top=N out of args. When absent, DefaultTopN is used.
func parseTopN(args []string) (int, error) {
	topN := DefaultTopN
	for _, arg := range args {
		if !strings.HasPrefix(arg, flagTopPrefix) {
			continue
		}
		raw := strings.TrimPrefix(arg, flagTopPrefix)
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			return 0, fmt.Errorf("--top requires a positive integer, got %q", raw)
		}
		topN = n
	}
	return topN, nil
}

// parseThreshold pulls --threshold=N out of args. When the flag is absent,
// DefaultThreshold is used.
func parseThreshold(args []string) (float64, error) {
	threshold := DefaultThreshold
	for _, arg := range args {
		if !strings.HasPrefix(arg, flagThresholdPrefix) {
			continue
		}
		raw := strings.TrimPrefix(arg, flagThresholdPrefix)
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil || v <= 0 {
			return 0, fmt.Errorf("--threshold requires a positive number, got %q", raw)
		}
		threshold = v
	}
	return threshold, nil
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
  --format=text|json|agent-json  Output format (default text)
  --top=N                    Hotspot limit for agent-json (default 6)
  --max-workers N            Analyze up to N modules in parallel (default: half the CPUs)
  --threshold=N              CRAP score above which the run fails (default 8.0)
`
}
