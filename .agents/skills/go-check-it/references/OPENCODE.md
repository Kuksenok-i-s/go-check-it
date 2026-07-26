# OpenCode + Ollama local subagents

This repository uses a shared OpenCode/Ollama bridge so Cursor, VS Code,
Claude Code, Codex, and OpenCode can all request focused local-model help
without changing each IDE's primary model.

## Setup

1. Install and start [Ollama](https://docs.ollama.com/linux).
2. Pull any local model you already trust (this project never auto-pulls).
3. From a go-check-it checkout, install host tools on `PATH` and OpenCode agents:

```sh
sh scripts/install-path.sh
setup-opencode
```

`setup-opencode` lists installed models (or uses `GO_CHECK_IT_LOCAL_MODEL`), then
creates/updates the stable alias `go-check-it-local` with `num_ctx 65536`.
OpenCode documents a 64K minimum context for repository work.

Optional:

```sh
setup-opencode --check     # non-mutating preflight
setup-opencode --install   # ollama launch opencode
```

`--install` may offer to install OpenCode through Ollama. It never installs
Cursor.

Verify the alias and runtime allocation:

```sh
ollama list
ollama ps
```

`opencode.json` registers `limit.context` for token accounting only. The real
context window comes from the Ollama alias parameter.

## Cross-IDE bridge

Every IDE should call (from any project directory):

```sh
run-local-subagent <role> --file /tmp/context.txt -- "your question"
```

Allowlisted roles:

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
```

## Optional swarm (bounded parallel scouts)

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
invalid input. The primary IDE agent synthesizes the envelope and owns all
edits. There is no automatic semantic merge and no invented token budget.

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
2. On the first failure, extract only that diagnostic and nearby code.
3. Call `run-local-subagent` with one role and that bounded context
   (or `run-local-swarm` for independent scouts).
4. Apply any accepted change in the primary IDE agent.
5. Rerun the full gate before declaring success.

Delegate one isolated failure or function at a time for fix loops. For
project scouting, keep each swarm task to a named shard — do not send the
whole repository to every worker.

## Trust boundary

Local subagents may read code and run narrowly allowlisted diagnostic
commands. Agent files use `mode: all` so they can be invoked both through
`run-local-subagent` / `opencode run --agent` and via `@name`
inside OpenCode. They must not:

- edit files;
- install packages;
- open network endpoints;
- recursively spawn other agents;
- decide the project is clean without a fresh gate run.

The primary IDE agent owns all edits and final verification. `run-local-swarm`
only orchestrates allowlisted leaf calls; it never widens permissions.

## Official references

- Ollama OpenCode integration: https://docs.ollama.com/integrations/opencode
- OpenCode providers: https://opencode.ai/docs/providers/#ollama
- OpenCode agents: https://opencode.ai/docs/agents/
- OpenCode skills discovery: https://opencode.ai/docs/skills/
- Context length: https://docs.ollama.com/context-length
