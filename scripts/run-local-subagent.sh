#!/usr/bin/env bash
# run-local-subagent.sh — IDE-neutral bridge to OpenCode/Ollama subagents.
#
# Every supported IDE (Cursor, VS Code, Claude Code, Codex, OpenCode) should
# call this script instead of configuring a native Ollama provider. It only
# invokes allowlisted read-only agents and never applies edits.

set -euo pipefail

ALLOWED_ROLES=(
	local-lint-diagnosis
	local-go-test-designer
	local-crap-refactor
	local-patch-review
)

usage() {
	cat <<'EOF' >&2
usage: sh scripts/run-local-subagent.sh <role> [--file PATH] [--] [prompt...]

Roles:
  local-lint-diagnosis
  local-go-test-designer
  local-crap-refactor
  local-patch-review

Options:
  --file PATH   Attach a context file (diagnostic dump, function excerpt, etc.)
  --            End of options; remaining args form the prompt

Environment:
  OPENCODE_BIN  Override the OpenCode executable path
EOF
	exit 2
}

role=${1:-}
[[ -n "$role" ]] || usage
shift

allowed=0
for r in "${ALLOWED_ROLES[@]}"; do
	if [[ "$role" == "$r" ]]; then
		allowed=1
		break
	fi
done
if [[ "$allowed" -ne 1 ]]; then
	echo "refusing unknown role: $role" >&2
	usage
fi

files=()
prompt_parts=()
while [[ $# -gt 0 ]]; do
	case "$1" in
		--file)
			[[ $# -ge 2 ]] || usage
			files+=("$2")
			shift 2
			;;
		--)
			shift
			prompt_parts+=("$@")
			break
			;;
		-h | --help)
			usage
			;;
		*)
			prompt_parts+=("$1")
			shift
			;;
	esac
done

if [[ ${#prompt_parts[@]} -eq 0 && ${#files[@]} -eq 0 ]]; then
	echo "a prompt or --file is required" >&2
	usage
fi

for f in "${files[@]+"${files[@]}"}"; do
	if [[ ! -f "$f" ]]; then
		echo "context file not found: $f" >&2
		exit 1
	fi
done

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(dirname -- "$script_dir")
cd "$repo_root"

resolve_opencode() {
	if [[ -n "${OPENCODE_BIN:-}" ]]; then
		echo "$OPENCODE_BIN"
		return 0
	fi
	if command -v opencode >/dev/null 2>&1; then
		command -v opencode
		return 0
	fi
	if [[ -x "${HOME}/.opencode/bin/opencode" ]]; then
		echo "${HOME}/.opencode/bin/opencode"
		return 0
	fi
	return 1
}

if ! bin=$(resolve_opencode); then
	echo "OpenCode is required but was not found on PATH" >&2
	echo "Run: sh scripts/setup-opencode.sh --install" >&2
	echo "Docs: https://opencode.ai/docs/" >&2
	exit 1
fi

if ! curl -fsS "${OLLAMA_HOST:-http://localhost:11434}/api/tags" >/dev/null 2>&1; then
	echo "Ollama API is not reachable. Start it with: ollama serve" >&2
	exit 1
fi

prompt=$(printf '%s ' "${prompt_parts[@]+"${prompt_parts[@]}"}")
prompt=${prompt%% }

cmd=("$bin" run --agent "$role" --format json)
for f in "${files[@]+"${files[@]}"}"; do
	cmd+=(--file "$f")
done
# Pin the local Ollama alias so the call does not depend on the IDE's primary model.
cmd+=(--model "ollama/go-check-it-local")
if [[ -n "$prompt" ]]; then
	cmd+=("$prompt")
fi

# Intentionally omit --auto: permissions stay deny-by-default on subagents.
exec "${cmd[@]}"
