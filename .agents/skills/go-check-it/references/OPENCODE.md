# OpenCode + Ollama / small-model subagents

This repository uses a shared OpenCode bridge so Cursor, VS Code, Claude Code,
Codex, and OpenCode can all request focused model help without changing each
IDE's primary model.

There are two tiers:

1. **Small non-local models** (`run-small-subagent` / `run-go-check-it-agents`)
   for dirty analysis and orchestration. Configure with
   `GO_CHECK_IT_SMALL_MODEL=provider/model`. Credentials stay in the caller's
   OpenCode config; go-check-it never writes secrets.
2. **Local Ollama models** (`run-local-subagent` / `run-local-swarm`) pinned to
   `ollama/go-check-it-local` for offline specialists.

## Setup

1. Install and start [Ollama](https://docs.ollama.com/linux) if you want local
   specialists.
2. Pull any local model you already trust (this project never auto-pulls).
3. Configure an OpenCode cloud/provider model for small non-local work and set:

```sh
export GO_CHECK_IT_SMALL_MODEL=provider/model   # e.g. anthropic/claude-haiku-4-5
```

4. From a go-check-it checkout, install host tools on `PATH` and OpenCode agents:

```sh
sh scripts/install-path.sh
setup-opencode
```

`setup-opencode` auto-recommends an installed model (preferring coder-oriented
names and ≥64K context), asks for confirmation, then creates/updates the stable
alias `go-check-it-local` with `num_ctx 65536`. Set `GO_CHECK_IT_LOCAL_MODEL` to
pin a model, or `GO_CHECK_IT_CONFIRM=1` to accept the recommendation non-interactively.
OpenCode documents a 64K minimum context for repository work.

Optional:

```sh
setup-opencode --check     # non-mutating preflight
setup-opencode --install   # ollama launch opencode
```

`--install` may offer to install OpenCode through Ollama. It never installs
Cursor.

Verify:

```sh
ollama list
ollama ps
command -v run-go-check-it-agents
command -v run-small-subagent
command -v run-local-subagent
```

`opencode.json` registers `limit.context` for token accounting only. The real
local context window comes from the Ollama alias parameter.

## Tiered pipeline (preferred)

Every IDE should call:

```sh
go-check-it --format=agent-json --top=6 --fail-on-findings > /tmp/agent.json || true
run-go-check-it-agents --agent-json /tmp/agent.json --max-workers 2
```

This:

- clusters CRAP hotspots into at most 6 `small-quality-worker` tasks;
- runs them via OpenCode with `GO_CHECK_IT_SMALL_MODEL`;
- asks `small-go-check-it-orchestrator` to emit a validated evidence packet;
- may let the orchestrator call allowlisted `local-*` specialists;
- writes raw OpenCode event logs under `raw_logs_dir` (opt-in debug only);
- never applies edits.

Single small-agent leaf:

```sh
run-small-subagent small-quality-worker --file /tmp/context.json -- "diagnose one hotspot"
run-small-subagent small-go-check-it-orchestrator --file /tmp/workers.json -- "synthesize packet"
```

## Local Ollama bridge

```sh
run-local-subagent <role> --file /tmp/context.txt -- "your question"
```

Allowlisted local roles:

| Role | Purpose |
|---|---|
| `local-lint-diagnosis` | Explain one fmt/vet/lint/test failure |
| `local-go-test-designer` | Propose focused table-driven tests |
| `local-crap-refactor` | Suggest a bounded CRAP/complexity fix |
| `local-patch-review` | Review the current git patch |
| `local-project-scout` | Scout a bounded aspect/shard; structured facts only |

The bridge:

- runs `opencode run --agent <role> --model ollama/go-check-it-local`;
- refuses unknown roles;
- never passes `--auto`;
- never applies edits (subagents are deny-by-default for write/network/task);
- stays in the caller's working directory (the project under review).

Inside OpenCode you can also invoke them manually:

```text
@local-lint-diagnosis diagnose the current lint failure
@local-go-test-designer design tests for ParseArgs
@local-crap-refactor analyze the highest-CRAP function
@local-patch-review review the current unstaged patch
@local-project-scout scout concurrency and cancellation paths
@small-quality-worker analyze one hotspot cluster
@small-go-check-it-orchestrator synthesize worker results
```

## Optional local swarm (bounded parallel scouts)

When **three or more independent** analysis areas exist (for example entry
points, concurrency, security/permissions, tests/docs), you may fan out with
`run-local-swarm` instead of sequential single-role calls. This is **opt-in**.

```sh
cat >/tmp/swarm-manifest.json <<'EOF'
{
  "tasks": [
    {
      "id": "entry",
      "role": "local-project-scout",
      "prompt": "Scout CLI/agent entry points only; list paths and symbols."
    },
    {
      "id": "concurrency",
      "role": "local-project-scout",
      "prompt": "Scout concurrency and cancellation; ignore unrelated packages."
    },
    {
      "id": "trust",
      "role": "local-project-scout",
      "prompt": "Scout permissions, allowlists, and trust boundaries only."
    }
  ]
}
EOF

run-local-swarm --manifest /tmp/swarm-manifest.json --max-workers 2
```

Flags:

- `--max-workers` default `2`, hard max `4`
- `--task-timeout` default `120` seconds
- `--total-timeout` default `300` seconds

The swarm prints one JSON envelope with results in manifest order. Exit `0`
only when every task succeeds; `1` means partial failure/timeout; `2` means
invalid input. Prefer `run-go-check-it-agents` for CRAP hotspot fan-out; use
`run-local-swarm` for orthogonal local scouts.

### When swarm is worth it

Use it when:

- you have **3+ orthogonal** shards (distinct packages or concerns);
- Ollama can actually run requests in parallel on your machine;
- wall-clock matters more than a single deep pass.

Skip it when:

- the question is one cross-cutting failure (prefer one strong sequential agent);
- the repository is small enough for a single scout;
- `ollama ps` / memory pressure shows requests queuing (64K context × workers
  multiplies KV-cache use). Start at `--max-workers 2` and raise only after
  checking memory and elapsed time.

## Delegation loop

1. Run the deterministic gate: `bash <skill-dir>/scripts/check.sh`.
2. On failure, prefer `run-go-check-it-agents` with `agent-json` when the small
   model is configured; otherwise extract one diagnostic and call
   `run-local-subagent`.
3. Primary IDE agent reviews the evidence packet / proposed diffs.
4. Apply accepted changes; run focused package/tool checks.
5. Run full `scripts/check.sh` once when candidate-clean.

Do not paste raw worker transcripts or full race stacks into the primary chat.
Use `raw_logs_dir` and race summaries instead.

## Trust boundary

Local and small subagents may read code and run narrowly allowlisted diagnostic
commands. Agent files use `mode: all` so they can be invoked both through the
bridges / `opencode run --agent` and via `@name` inside OpenCode. They must not:

- edit files;
- install packages;
- open network endpoints (except the model provider already configured in OpenCode);
- recursively spawn unlimited agents (`small-quality-worker` cannot task at all;
  the orchestrator may only call allowlisted `local-*` leaves);
- decide the project is clean without a fresh gate run.

The primary IDE agent owns all edits and final verification.
`run-go-check-it-agents` / `run-local-swarm` only orchestrate allowlisted leaf
calls; they never widen permissions.

## Official references

- Ollama OpenCode integration: https://docs.ollama.com/integrations/opencode
- OpenCode providers: https://opencode.ai/docs/providers/#ollama
- OpenCode agents: https://opencode.ai/docs/agents/
- OpenCode skills discovery: https://opencode.ai/docs/skills/
- Context length: https://docs.ollama.com/context-length
