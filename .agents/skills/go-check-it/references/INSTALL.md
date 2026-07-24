# Installing go-check-it

The skill and the `go-check-it` executable are separate:

1. install the executable;
2. expose the skill directory to your agent.

## Install go-check-it

From a checkout of this repository:

```sh
go install ./cmd/go-check-it
```

Ensure `$(go env GOPATH)/bin` (or `GOBIN`) is on `PATH`, then verify:

```sh
go-check-it --help
```

When developing this repository, the skill builds `./cmd/go-check-it` directly and
does not require a global installation.

## Repository scope

The canonical skill is checked in at:

```text
.agents/skills/go-check-it/
```

- **Cursor:** discovers `.agents/skills/` automatically; this repository also
  exposes the same canonical directory through `.cursor/skills/`.
- **VS Code with GitHub Copilot:** discovers `.agents/skills/` automatically.
- **Codex:** discovers `.agents/skills/` from the working directory through the
  repository root automatically.
- **Claude Code:** this repository includes a small
  `.claude/skills/go-check-it/` bridge to the canonical skill.

Restart the agent if it was already running before the skill directory was
created. Invoke it explicitly as `/go-check-it` where slash skills are
supported, or ask the agent to run the Go quality workflow.

## Personal scope

From this repository, run:

```sh
sh scripts/install-agent-skill.sh cursor
sh scripts/install-agent-skill.sh vscode
sh scripts/install-agent-skill.sh claude
sh scripts/install-agent-skill.sh codex
```

Each command installs a copy for one user profile:

| Agent | Personal skill directory |
|---|---|
| Cursor | `~/.cursor/skills/go-check-it/` |
| VS Code / GitHub Copilot | `~/.copilot/skills/go-check-it/` |
| Claude Code | `~/.claude/skills/go-check-it/` |
| Codex | `~/.agents/skills/go-check-it/` |

If the destination already exists, the installer stops without changing it.
Pass `--force` to replace only that installed `go-check-it` directory:

```sh
sh scripts/install-agent-skill.sh cursor --force
```

## Local Ollama / OpenCode (all IDEs)

Local-model help for small diagnostics is shared across Cursor, VS Code,
Claude Code, Codex, and OpenCode through OpenCode + Ollama. Primary IDE models
stay unchanged.

```sh
sh scripts/setup-opencode.sh
sh scripts/run-local-subagent.sh local-lint-diagnosis -- "diagnose the first failing check"
```

See [OPENCODE.md](OPENCODE.md) for model selection, the 64K alias, roles, and
trust boundaries.

## Official references

- Cursor: https://cursor.com/docs/skills
- VS Code: https://code.visualstudio.com/docs/agent-customization/agent-skills
- Claude Code: https://code.claude.com/docs/en/skills
- Codex: https://developers.openai.com/codex/skills
- OpenCode + Ollama: https://docs.ollama.com/integrations/opencode
