---
name: go-check-it
description: Runs and fixes a complete Go quality loop using formatting, tests, race detection, go vet, golangci-lint, and the installed go-check-it binary for CRAP/practice checks. Prefers compact agent-json and optional small-model delegation. Use when writing or reviewing Go code, checking Go project quality, fixing lint or test failures, or preparing Go changes for CI. Never copies or ports go-check-it into the target repository.
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

Default loop:

1. Run the deterministic gate once (`scripts/check.sh` or the steps below).
2. If the bridge is configured, delegate bounded analysis (see §6).
3. Review/apply the evidence packet yourself (primary agent owns edits).
4. Run the smallest relevant package/tool check for the change.
5. Repeat compact analysis as needed.
6. When candidate-clean, run full `scripts/check.sh` **once**. Rerun it again
   only if that final gate reveals a new class of failure.

Do **not** rerun full `check.sh` after every delegated or local fix. Use
focused checks while iterating.

## 1. Establish scope

Work from the Go workspace or module root. Preserve unrelated user changes.
Inspect the repository's `go.mod`, existing lint configuration, and documented
test commands before running checks.

For a focused edit, run the narrow affected package tests while iterating, but
finish with the complete workflow below.

## 2. Fast correctness loop

1. Run `gofmt -l .`. Format every listed Go file with `gofmt -w`.
2. Run `go test ./...` (or the affected package while iterating).
3. Run `go vet ./...`.
4. Fix failures and repeat these steps until clean.

Do not hide failures with `//nolint`, skipped tests, or weakened assertions
unless the user explicitly accepts the trade-off.

## 3. Race and lint checks

1. Run `go test -race ./...` (or the affected package with `-race` while fixing).
2. Run `golangci-lint run ./...` when the repository has a golangci-lint
   configuration or the command is available.
3. If golangci-lint is unavailable, report that clearly and use its official
   installation instructions. Do not invent a version.
4. Fix findings and rerun the relevant checks.

### Race output policy

Race detector stacks are large and easy to mishandle:

1. Preserve the **complete** failing race log outside chat (temp file or
   `raw_logs_dir`).
2. Give workers/yourself only a deterministic summary: failing test name,
   competing Read/Write accesses (file:line + function), goroutine creation
   locations, and race count. The helper
   `scripts/lib/race_summary.py` can build this summary.
3. While fixing, rerun focused `go test -race` on the affected package
   repeatedly (prefer several successes, not one flaky pass).
4. Require the final repository-wide `go test -race ./...` via `check.sh`.
5. Never declare a race fixed from a single non-reproducing retry.

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

Default agent command:

```sh
go-check-it --format=agent-json --top=6 --fail-on-findings
```

Use `/tmp/go-check-it` instead of `go-check-it` when step 1 built the local binary.

`agent-json` returns a compact summary, truncated CRAP hotspots, practice
findings (with why/fix/docUrl), and omitted counts. Use
`--format=json` only for archival/debug when the full functions table is
explicitly needed.

Treat these outcomes as failures to fix:

- exit `2`: at least one function has CRAP greater than the threshold
  (default `8.0`; override with `--threshold=N` if the repo's own docs say
  to use a different value — don't invent one yourself);
- exit `3`: one or more Go practice findings were reported;
- exit `1`: usage, tooling, cancellation, or coverage infrastructure failed.

For high CRAP, fix it with tests or same-package helpers, never a new package
per helper — strict rules and an example: [REFACTORING.md](references/REFACTORING.md).

For practice findings, follow the explanation already embedded in JSON
(`summary`, `why`, `fix`, `docUrl`). **Do not read**
[RULES.md](references/RULES.md) when that JSON is available. Read RULES.md
only when the tool is unavailable or the JSON is incomplete.

## 5. Repeat to completion

After every fix, rerun the smallest relevant check first (package tests,
focused `-race`, single lint, or scoped `go-check-it <path>`). Before declaring
the work complete, rerun `scripts/check.sh`, which covers all of:

```sh
gofmt -l .
go test ./...
go test -race ./...
go vet ./...
golangci-lint run ./...
go-check-it --format=agent-json --top=6 --fail-on-findings
```

The final handoff must state:

- which checks passed;
- maximum CRAP score and function, when available;
- practice-finding count;
- any check that could not run and the exact reason;
- whether small-model delegation was used.

## 6. Optional tiered small-model delegation

Prefer small models for dirty analysis. The primary IDE model should review a
compact evidence packet, not raw worker transcripts.

### Cross-IDE pipeline (preferred)

When `run-go-check-it-agents` is on `PATH` and `GO_CHECK_IT_SMALL_MODEL` is set
(`provider/model`, for example `anthropic/claude-haiku-4-5`):

```sh
go-check-it --format=agent-json --top=6 --fail-on-findings > /tmp/agent.json || true
run-go-check-it-agents --agent-json /tmp/agent.json --max-workers 2
```

The pipeline:

1. Clusters hotspots into at most 6 non-local `small-quality-worker` tasks.
2. Fans them out through OpenCode using `GO_CHECK_IT_SMALL_MODEL`.
3. Asks `small-go-check-it-orchestrator` to synthesize a validated evidence
   packet (proposed diffs, confidence, disagreements, focused commands,
   escalate reasons).
4. May consult pinned local Ollama `local-*` specialists from the orchestrator.
5. Stores raw OpenCode logs under `raw_logs_dir` for opt-in debugging only —
   **do not** read that directory into primary context by default.

Escalate to yourself (primary) instead of auto-applying when the packet status
is `escalate`/`partial`, or when work involves races/concurrency, public API
changes, security/auth, cross-package behavior, worker disagreement, low
confidence, or unverified expected behavior.

Workers and the orchestrator are read-only. Apply accepted diffs yourself, then
run focused checks. Skip the pipeline and say so if
`run-go-check-it-agents` or `GO_CHECK_IT_SMALL_MODEL` is missing.

### Local-only leaf (fallback)

If only local Ollama is available:

```sh
run-local-subagent <role> --file /tmp/fn.go -- "your question"
```

Roles: `local-lint-diagnosis`, `local-go-test-designer`, `local-crap-refactor`,
`local-patch-review`, `local-project-scout`. Rules:

1. One isolated failure or function per call.
2. Local subagents are read-only — apply accepted changes yourself.
3. Rerun the smallest relevant check after every applied change; full
   `scripts/check.sh` only when candidate-clean.
4. Skip delegation and say so if `run-local-subagent` is missing. Never copy
   these scripts into the repository under review.

Optional swarm of independent local scouts: `run-local-swarm` (see
[OPENCODE.md](references/OPENCODE.md)).

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
- Prefer compact `agent-json` and evidence packets over pasting full check logs
  into chat.

See [installation](references/INSTALL.md) for Cursor, VS Code, Claude Code, and
Codex setup, and [OPENCODE.md](references/OPENCODE.md) for Ollama/OpenCode and
the small-model pipeline.
