---
name: go-check-it
description: Runs and fixes a complete Go quality loop using formatting, tests, race detection, go vet, golangci-lint, and go-check-it CRAP/practice checks. Use when writing or reviewing Go code, checking Go project quality, fixing lint or test failures, or preparing Go changes for CI.
---

# Go Check It

Run the checks below, fix every actionable failure, and repeat until the full
workflow is clean. Do not stop after the first successful partial check.

## Preferred entry point

Run `bash <skill-dir>/scripts/check.sh` from the repository being checked,
resolving `<skill-dir>` relative to this `SKILL.md`. The script executes the
complete fail-fast gate using the repository's own `cmd/go-check-it` when present,
otherwise the `go-check-it` executable on `PATH`.

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

1. If the current repository contains `cmd/go-check-it`, build it:
   `go build -o /tmp/go-check-it ./cmd/go-check-it`.
2. Otherwise, use `go-check-it` from `PATH`.
3. If neither is available, complete the other checks and report that CRAP and
   practice analysis could not run. Point to the installation instructions
   shipped with this skill.

Run:

```sh
go-check-it --explain --format=json --fail-on-findings
```

Use `/tmp/go-check-it` instead of `go-check-it` when step 1 built the local binary.

Treat these outcomes as failures to fix:

- exit `2`: at least one function has CRAP greater than `8.0`;
- exit `3`: one or more Go practice findings were reported;
- exit `1`: usage, tooling, cancellation, or coverage infrastructure failed.

For high CRAP, prefer focused tests that increase meaningful branch coverage,
or split genuinely multi-purpose functions. Do not add superficial tests solely
to manipulate the score.

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

When a single diagnostic or function is hard to fix by hand, and Ollama plus
OpenCode are configured, delegate through the shared bridge — not through an
IDE-specific model provider:

```sh
sh scripts/run-local-subagent.sh local-lint-diagnosis --file /tmp/diag.txt -- "explain and propose the smallest fix"
sh scripts/run-local-subagent.sh local-go-test-designer --file /tmp/fn.go -- "design focused table-driven tests"
sh scripts/run-local-subagent.sh local-crap-refactor --file /tmp/fn.go -- "suggest a bounded CRAP reduction"
sh scripts/run-local-subagent.sh local-patch-review -- "review the current unstaged patch"
```

Rules:

1. Delegate only one isolated failure or function at a time.
2. Keep the primary IDE model unchanged; the bridge pins `ollama/go-check-it-local`.
3. Apply accepted changes yourself (or with the primary agent). Local subagents
   are read-only.
4. Rerun `scripts/check.sh` after every applied change.

If the bridge is unavailable, continue with the deterministic gates above and
report that local delegation was skipped. Setup details are in
[OPENCODE.md](references/OPENCODE.md).

## Judgment

- Keep fixes narrow and idiomatic.
- Treat `goroutinelifetime` confidence as a review signal, not proof of a leak.
- If a heuristic finding is incorrect for the code, explain why instead of
  suppressing it broadly.
- Never claim a clean run from stale output; rerun after the final edit.

See [installation](references/INSTALL.md) for Cursor, VS Code, Claude Code, and
Codex setup, and [OPENCODE.md](references/OPENCODE.md) for Ollama/OpenCode.
