---
name: go-check-it
description: Runs and fixes a complete Go quality loop using formatting, tests, race detection, go vet, golangci-lint, and the installed go-check-it binary for CRAP/practice checks. Use when writing or reviewing Go code, checking Go project quality, fixing lint or test failures, or preparing Go changes for CI. Never copies or ports go-check-it into the target repository.
---

# Go Check It

Run the checks below, fix every actionable failure, and repeat until the full
workflow is clean. Do not stop after the first successful partial check.

## Hard rules (consumer repositories)

`go-check-it` is an external tool, like `gofmt` or `golangci-lint`.

- Prefer the `go-check-it` executable already on `PATH`.
- Build `./cmd/go-check-it` only when you are working inside the go-check-it
  source repository itself (module path `go-check-it` already contains that
  command).
- Never create, copy, vendor, symlink, or port `cmd/go-check-it`,
  `cmd/crap4go`, or any go-check-it / crap4go / crap4java source tree into the
  repository being checked.
- If `go-check-it` is missing from `PATH` and this is not the go-check-it
  source repo, finish the other gates and tell the user to install the binary
  globally (see [INSTALL.md](references/INSTALL.md)). Do not implement a
  substitute analyzer.

## Preferred entry point

Run `bash <skill-dir>/scripts/check.sh` from the repository being checked,
resolving `<skill-dir>` relative to this `SKILL.md`. The script builds
`./cmd/go-check-it` only inside the go-check-it source repository; everywhere
else it requires `go-check-it` on `PATH`.

Fix the first failing gate, then rerun the script from the beginning. Continue
until it exits successfully. Use the individual steps below for diagnosis and
focused iteration.

## 1. Establish scope

Work from the Go workspace or module root. Preserve unrelated user changes.
Inspect the repository's `go.mod`, existing lint configuration, and documented
test commands before running checks.

For a focused edit, run the narrow affected package tests while iterating, but
finish with the complete workflow below.

## 2. Fast correctness loop

1. Run `gofmt -l .`. Format every listed Go file with `gofmt -w`.
2. Run `go test ./...`.
3. Run `go vet ./...`.
4. Fix failures and repeat these steps until clean.

Do not hide failures with `//nolint`, skipped tests, or weakened assertions
unless the user explicitly accepts the trade-off.

## 3. Race and lint checks

1. Run `go test -race ./...`.
2. Run `golangci-lint run ./...` when the repository has a golangci-lint
   configuration or the command is available.
3. If golangci-lint is unavailable, report that clearly and use its official
   installation instructions. Do not invent a version.
4. Fix findings and rerun both checks.

## 4. Run go-check-it

Resolve the executable in this order:

1. If this repository is the go-check-it source tree (`go.mod` module
   `go-check-it` and `./cmd/go-check-it` already exists), build it:
   `go build -o /tmp/go-check-it ./cmd/go-check-it`.
2. Otherwise use `go-check-it` from `PATH`.
3. If neither applies, complete the other checks and report that CRAP and
   practice analysis could not run. Point to
   [INSTALL.md](references/INSTALL.md). Do not add the tool to the repo.
   If you find a stray `cmd/go-check-it` or `cmd/crap4go` in a non-source
   repo, treat that as a mistake to remove — not something to build.

Run:

```sh
go-check-it --explain --format=json --fail-on-findings
```

Use `/tmp/go-check-it` instead of `go-check-it` when step 1 built the local binary.

Treat these outcomes as failures to fix:

- exit `2`: at least one function has CRAP greater than `8.0`;
- exit `3`: one or more Go practice findings were reported;
- exit `1`: usage, tooling, cancellation, or coverage infrastructure failed.

For high CRAP, fix it with tests or same-package helpers, never a new package
per helper — strict rules and an example: [REFACTORING.md](references/REFACTORING.md).

For practice findings, follow the explanation in JSON. Read
[the rule catalog](references/RULES.md) only when the tool is unavailable or
when more context is needed.

## 5. Repeat to completion

After every fix, rerun the smallest relevant check first. Before declaring the
work complete, rerun `scripts/check.sh`, which covers all of:

```sh
gofmt -l .
go test ./...
go test -race ./...
go vet ./...
golangci-lint run ./...
go-check-it --explain --format=json --fail-on-findings
```

The final handoff must state:

- which checks passed;
- maximum CRAP score and function, when available;
- practice-finding count;
- any check that could not run and the exact reason.

## 6. Optional local-model delegation (OpenCode + Ollama)

If a single diagnostic or function is hard to fix by hand and `run-local-subagent`
is on `PATH`, delegate one bounded task through the shared bridge instead of
an IDE-specific model provider:

```sh
run-local-subagent <role> --file /tmp/fn.go -- "your question"
```

Roles: `local-lint-diagnosis`, `local-go-test-designer`, `local-crap-refactor`,
`local-patch-review`, `local-project-scout`. Rules:

1. One isolated failure or function per call.
2. Local subagents are read-only — apply accepted changes yourself.
3. Rerun `scripts/check.sh` after every applied change.
4. Skip delegation and say so if `run-local-subagent` is missing. Never copy
   these scripts into the repository under review.

For the setup steps, the swarm variant (`run-local-swarm`, 3+ independent
scouts), and the trust boundary: [OPENCODE.md](references/OPENCODE.md).

## Judgment

- Keep fixes narrow and idiomatic.
- Avoid over-fragmentation: extracting helper functions is fine, but don't
  spin up a new package per helper — see [REFACTORING.md](references/REFACTORING.md).
- Treat `goroutinelifetime` confidence as a review signal, not proof of a leak.
- If a heuristic finding is incorrect for the code, explain why instead of
  suppressing it broadly.
- Never claim a clean run from stale output; rerun after the final edit.
- Never treat a missing `go-check-it` binary as a reason to add source to the
  project under review.

See [installation](references/INSTALL.md) for Cursor, VS Code, Claude Code, and
Codex setup, and [OPENCODE.md](references/OPENCODE.md) for Ollama/OpenCode.
