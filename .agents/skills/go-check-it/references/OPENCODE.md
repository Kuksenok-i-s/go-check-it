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
```

## Delegation loop

1. Run the deterministic gate: `bash <skill-dir>/scripts/check.sh`.
2. On the first failure, extract only that diagnostic and nearby code.
3. Call `run-local-subagent` with one role and that bounded context.
4. Apply any accepted change in the primary IDE agent.
5. Rerun the full gate before declaring success.

Delegate one isolated failure or function at a time. Do not send the whole
repository to the local model.

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

The primary IDE agent owns all edits and final verification.

## Official references

- Ollama OpenCode integration: https://docs.ollama.com/integrations/opencode
- OpenCode providers: https://opencode.ai/docs/providers/#ollama
- OpenCode agents: https://opencode.ai/docs/agents/
- OpenCode skills discovery: https://opencode.ai/docs/skills/
- Context length: https://docs.ollama.com/context-length
