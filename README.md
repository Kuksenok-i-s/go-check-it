# go-check-it

`go-check-it` is a standalone CRAP metric tool for Go projects, inspired by
[`crap4go`](https://github.com/unclebob/crap4go) and
[`crap4java`](https://github.com/unclebob/crap4java) (itself modeled after
`crap4clj`), implemented in idiomatic Go.

It combines per-function cyclomatic complexity with `go test` coverage and
reports CRAP scores. On each run it generates a coverage profile via `go
test -coverprofile`, then analyzes the selected files.

## Formula

```
CRAP = CC^2 * (1 - coverage)^3 + CC
```

* `CC` is cyclomatic complexity.
* `coverage` is the function's covered-statement fraction, taken from the Go
  coverage profile.

Low CRAP scores mean low change/maintenance risk (the function is simple
and/or well tested). High CRAP scores mean a function is both complex *and*
poorly tested — the combination that makes code risky to change.

## Coverage Pipeline

For each Go module found among the selected files:

1. Run `go test ./... -coverprofile=<tmp file>` in that module's root
   (the directory containing its `go.mod`).
2. Parse the resulting coverage profile.
3. Attribute each function's statements to it by line range (no name
   matching needed — Go's coverage profile is already line-accurate).
4. Delete the temporary profile file.

If `go test` can't even start (e.g. the `go` toolchain is missing), the run
fails fast. If it runs but some tests fail, whatever coverage was recorded is
still used, with a warning. If no coverage profile is produced at all,
affected functions are reported with `N/A` coverage/CRAP instead of failing
the whole run.

## Build, Test, and Lint

```
go build ./...
go test ./...
gofmt -l .                          # should print nothing
go vet ./...
golangci-lint run                   # see .golangci.yml
```

`golangci-lint` isn't part of the Go toolchain; install it per the
[official instructions](https://golangci-lint.run/docs/welcome/install/) if
you don't have it.

## Run

```
go build -o go-check-it ./cmd/go-check-it
./go-check-it
```

## Agent Skill

This repository includes a portable `go-check-it` Agent Skill. It teaches an
agent to run and fix the complete Go quality loop: formatting, tests, race
detection, `go vet`, golangci-lint, CRAP scoring, and the built-in practice
checks. It repeats the workflow after fixes and only reports success from a
fresh clean run.

The skill includes `scripts/check.sh`, a deterministic fail-fast gate that runs
the same workflow in Cursor, VS Code, Claude Code, Codex, and OpenCode.

The canonical skill lives at:

```text
.agents/skills/go-check-it/SKILL.md
```

### Project use

After cloning this repository:

| Agent | Setup |
|---|---|
| Cursor | Automatic from `.agents/skills/`; `.cursor/skills/` also links to the canonical skill |
| VS Code / GitHub Copilot | Automatic from `.agents/skills/` |
| Claude Code | Automatic through the committed `.claude/skills/` bridge |
| Codex | Automatic from `.agents/skills/` |
| OpenCode | Automatic from `.agents/skills/` plus project `opencode.json` / `.opencode/agents/` |

Invoke `/go-check-it` where slash skills are supported, or ask the agent to run
the Go quality workflow.

### Local Ollama subagents (shared by all IDEs)

Focused local-model help uses a shared OpenCode bridge. No IDE-specific Ollama
provider or primary-model override is required.

```sh
# 1) Select an already-installed Ollama model and create go-check-it-local (64K)
sh scripts/setup-opencode.sh

# 2) Optional: launch / install OpenCode via Ollama (never installs Cursor)
sh scripts/setup-opencode.sh --install

# 3) From any IDE agent, delegate one bounded task:
sh scripts/run-local-subagent.sh local-lint-diagnosis --file /tmp/diag.txt -- "smallest fix?"
sh scripts/run-local-subagent.sh local-go-test-designer --file internal/cliapp/args.go -- "tests for ParseArgs"
sh scripts/run-local-subagent.sh local-crap-refactor -- "reduce CRAP for the top function"
sh scripts/run-local-subagent.sh local-patch-review -- "review the current unstaged patch"
```

Inside OpenCode you can also use `@local-lint-diagnosis`, `@local-go-test-designer`,
`@local-crap-refactor`, or `@local-patch-review`. Local subagents are read-only;
the primary IDE agent applies edits and must rerun `scripts/check.sh`.

Details: [OPENCODE.md](.agents/skills/go-check-it/references/OPENCODE.md).

### Personal installation

Install the skill for one agent across all projects:

```sh
sh scripts/install-agent-skill.sh cursor
sh scripts/install-agent-skill.sh vscode
sh scripts/install-agent-skill.sh claude
sh scripts/install-agent-skill.sh codex
```

The installer refuses to overwrite an existing installation. Add `--force` to
replace only the existing `go-check-it` skill directory.

Install the executable separately from this checkout:

```sh
go install ./cmd/go-check-it
```

See the skill's
[installation reference](.agents/skills/go-check-it/references/INSTALL.md) for
personal paths, verification, and official platform documentation.

## CLI

```
--help                Print usage to stdout
(no args)             Analyze all *.go files in the project (excluding vendor/, hidden, and testdata dirs)
--changed             Analyze changed *.go files (via `git status --porcelain`)
<file ...>            Analyze only these files
<directory ...>       Analyze all *.go files under each directory

--explain             Print each practice finding's rationale, fix, and doc link
--fail-on-findings    Exit non-zero if any practice finding is reported
--format=text|json    Output format (default text); JSON always includes the
                      full explanation for every finding
--max-workers N       Analyze up to N Go modules in parallel (default: half
                      the logical CPUs, at least 1). Caps at the number of
                      discovered modules. File discovery and practice checks
                      remain sequential.
```

Examples:

```
./go-check-it --help
./go-check-it
./go-check-it --changed
./go-check-it --max-workers 4
./go-check-it internal/server/handler.go
./go-check-it internal/server internal/store
./go-check-it --explain internal/server
./go-check-it --format=json --fail-on-findings
```

## Exit codes

* `0` success, threshold respected (or nothing to analyze)
* `1` invalid CLI usage, an infra-level failure (e.g. `go` not found), or the
  run was interrupted (e.g. Ctrl-C) while generating coverage
* `2` CRAP threshold exceeded (`> 8.0`)
* `3` `--fail-on-findings` was set and at least one practice finding was
  reported

## Practice Checks

Alongside the CRAP score, go-check-it runs a curated set of `go/analysis`-shaped
checks for Go best practices that CRAP doesn't capture, printed as a
"Practice Findings" section (or under `"findings"` in `--format=json`).
Each finding carries a structured explanation (why it matters, a concrete
fix, and an official doc link) — shown with `--explain`, always present in
JSON — so an agent reading the report can act on it without a side lookup.

| Rule | What it flags |
|------|---------------|
| `panicinmain` | `panic()` in package main; return an error and let main log + `os.Exit(1)` |
| `errorstrings` | error strings that are capitalized or end with punctuation |
| `doccomment` | exported decl doc comments that don't start with the name or end with a period |
| `contextprop` | `exec.Command`/`http.Get` etc. that ignore an available context-aware variant |
| `interfaceownership` | interfaces defined beside their own implementation with no local consumer |
| `receivername` | inconsistent receiver names across a type's methods |
| `nakedreturn` | naked `return` in functions longer than 20 lines with named results |
| `initialism` | wrongly cased initialisms (`Id`, `Url`, `Http`, ...) |
| `underscorename` | `snake_case` package-level names instead of MixedCaps |
| `goroutinelifetime` | `go func(){...}()` with no visible exit signal, confidence-scored 0-10 |

Design trade-off: these checks parse files with `go/parser`/`go/ast` only —
like the rest of go-check-it, they never require a full type-checked build via
`go/packages`. That keeps them fast and able to run over code that doesn't
fully build, at the cost of a few rules (notably `interfaceownership` and
`goroutinelifetime`) being syntactic heuristics rather than soundly verified.
`goroutinelifetime` in particular can't be verified statically at all, so
each finding self-reports a 0-10 confidence instead of a bare pass/fail, and
only launches scoring above the neutral midpoint are reported. These checks
are informational by default; use `--fail-on-findings` to enforce them in CI.

## Official Go Guidance Applied

Beyond `gofmt`/`go vet` (checked on every build), this project follows
[Effective Go](https://go.dev/doc/effective_go) and the
[Go Wiki: Code Review Comments](https://go.dev/wiki/CodeReviewComments)
checklist. A few points worth calling out explicitly:

* **Interfaces belong to the consumer.** `CoverageGenerator` is declared in
  `internal/cliapp` (which calls it), not in `internal/coverage` (which
  implements it) — see the wiki's "Interfaces" entry.
* **Context propagation and cancellation.** `App.Execute` takes a
  `context.Context` as its first parameter and threads it down to every
  subprocess call (`go test`, `git status`) via `exec.CommandContext`, so
  Ctrl-C kills those children immediately instead of leaving the tool to
  wait on them. `main` wires this up with `signal.NotifyContext`. See the
  wiki's "Contexts" entry.
* **No `panic` for ordinary errors.** `main` reports startup failures (e.g.
  resolving the working directory) to stderr and exits with a non-zero
  status, rather than panicking — see the wiki's "Don't Panic" entry.
* **Full-sentence doc comments and lowercase error strings**, per the
  wiki's "Comment Sentences" and "Error Strings" entries; enforced going
  forward by the `godot`/`revive` linters below *and* by go-check-it's own
  `doccomment`/`errorstrings` checks (see "Practice Checks" above).

`golangci-lint` (config: `.golangci.yml`) enables a curated, low-noise set of
linters on top of the standard set (`errcheck`, `govet`, `staticcheck`,
`unused`, `ineffassign`) to keep these guidelines from regressing:
`contextcheck` and `errorlint` for the context/error-wrapping points above,
`godot`/`revive` for comment and naming style, plus `gocritic`, `goconst`,
`misspell`, `noctx`, `unconvert`, and `unparam` for general correctness and
clarity.

## Notes

* If a module has no coverage profile, coverage is reported as `N/A` (and so
  is CRAP, since it's undefined without a coverage measurement).
* The report is sorted by CRAP descending, with `N/A` sorted last.
* Cyclomatic complexity starts at 1 per function and is incremented for
  `if`, `for`/`range`, each `switch`/`select` case (including `default`),
  and each short-circuit `&&`/`||`. Control flow inside a function literal
  (closure) folds into the complexity of its enclosing function, since
  closures aren't independently addressable units in Go.
* Interface method declarations (no body) aren't analyzed, since there's no
  body to measure.

## Differences from crap4java

This is a deliberate, idiomatic port rather than a line-for-line
translation. The differences follow from how Go projects are structured and
tested, compared to Maven/JaCoCo-based Java projects:

| crap4java (Java/Maven)                          | go-check-it (Go)                                       |
|--------------------------------------------------|-----------------------------------------------------|
| Source lives under `src/`                        | Source lives at the module root; `vendor/`, hidden, and `testdata/` dirs are skipped instead |
| Module root = nearest `pom.xml`                  | Module root = nearest `go.mod`                       |
| Coverage via `mvn ... jacoco:report` + XML       | Coverage via `go test -coverprofile` + profile parsing |
| Coverage attributed by class/method name + nearest line | Coverage attributed by exact statement line ranges (no name matching needed) |
| Parses via the JDK compiler tree API             | Parses via `go/parser`/`go/ast`                       |
| Excludes constructors, abstract methods, anonymous-class methods | Excludes bodiless declarations (interface methods); closures fold into their enclosing function instead of being excluded outright |

The CRAP formula, default threshold (`8.0`), report ordering, and exit codes
(`0`/`1`/`2`) are unchanged from `crap4java`.

## Project Layout

```
cmd/go-check-it/          entry point
internal/cliapp/       CLI argument parsing + orchestration
internal/sourcefind/   "analyze everything" file discovery
internal/changed/      "--changed" file discovery via git
internal/modroot/      go.mod-based module grouping
internal/complexity/   go/ast-based cyclomatic complexity + function extraction
internal/coverage/     `go test -coverprofile` runner + profile parser
internal/crap/         the CRAP score formula
internal/analyzer/     combines complexity + coverage into per-function metrics
internal/report/       tabular report formatting, JSON output + threshold check
internal/practices/    go/analysis-shaped best-practice checks (see "Practice Checks")
  contextprop/         context propagation for exec/http calls
  doccomment/          doc comment start/end style
  errorstrings/        error string casing/punctuation
  goroutinelifetime/   goroutine exit-signal heuristic (confidence-scored)
  initialism/          initialism casing
  interfaceownership/  producer-side interface heuristic
  nakedreturn/         naked returns in long functions
  panicinmain/         panic() in package main
  receivername/        receiver name consistency
  underscorename/      snake_case names
```
