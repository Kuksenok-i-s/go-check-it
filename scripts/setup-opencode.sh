#!/usr/bin/env bash
# setup-opencode.sh — configure a stable Ollama alias for OpenCode subagents.
#
# Default mode is non-mutating aside from creating/updating the local alias
# go-check-it-local. Pass --check to validate only. Pass --install to use
# ollama launch opencode (may offer to install OpenCode). Never pulls models
# and never installs or invokes Cursor.

set -euo pipefail

ALIAS_NAME="go-check-it-local"
MIN_CTX=65536
OLLAMA_HOST="${OLLAMA_HOST:-http://localhost:11434}"

mode="setup"
while [[ $# -gt 0 ]]; do
	case "$1" in
		--check)
			mode="check"
			shift
			;;
		--install)
			mode="install"
			shift
			;;
		-h | --help)
			cat <<'EOF'
usage: setup-opencode [--check|--install]

  (default)  Select an installed Ollama model and create/update go-check-it-local
             with a 64K context window for OpenCode subagents.
  --check    Validate ollama, OpenCode, and the go-check-it-local alias only.
  --install  Run `ollama launch opencode` (may offer to install OpenCode).

Environment:
  GO_CHECK_IT_LOCAL_MODEL   Skip interactive selection; use this installed model.
  OLLAMA_HOST               Ollama API base (default http://localhost:11434).

Install on PATH (once):
  sh scripts/install-path.sh
EOF
			exit 0
			;;
		*)
			echo "unknown option: $1" >&2
			exit 2
			;;
	esac
done

require_cmd() {
	if ! command -v "$1" >/dev/null 2>&1; then
		echo "$1 is required but was not found on PATH" >&2
		return 1
	fi
}

opencode_bin() {
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

ensure_ollama() {
	require_cmd ollama || {
		echo "Install Ollama: https://docs.ollama.com/linux" >&2
		exit 1
	}
	if ! curl -fsS "${OLLAMA_HOST}/api/tags" >/dev/null 2>&1; then
		echo "Ollama API is not reachable at ${OLLAMA_HOST}" >&2
		echo "Start it with: ollama serve" >&2
		exit 1
	fi
}

list_models() {
	# Prefer API JSON; fall back to `ollama list` first column.
	# Normalize bare names and name:latest so alias checks match either form.
	if command -v python3 >/dev/null 2>&1; then
		curl -fsS "${OLLAMA_HOST}/api/tags" | python3 -c '
import json,sys
data=json.load(sys.stdin)
seen=set()
for m in data.get("models") or []:
    name=m.get("name") or m.get("model")
    if not name:
        continue
    for candidate in (name, name.split(":",1)[0]):
        if candidate not in seen:
            seen.add(candidate)
            print(candidate)
'
		return 0
	fi
	ollama list 2>/dev/null | awk 'NR>1 {
		print $1
		split($1, a, ":")
		if (a[1] != $1) print a[1]
	}'
}

model_context_limit() {
	local model=$1
	# Try `ollama show` PARAMETER/num_ctx or model info.
	local shown
	shown=$(ollama show "$model" 2>/dev/null || true)
	local ctx
	ctx=$(printf '%s\n' "$shown" | awk '
		tolower($0) ~ /context length/ { print $NF; exit }
		tolower($1) == "num_ctx" { print $2; exit }
	')
	if [[ -n "$ctx" && "$ctx" =~ ^[0-9]+$ ]]; then
		echo "$ctx"
		return 0
	fi
	# Unknown advertised limit: allow alias creation; OpenCode still needs 64K
	# at runtime via the alias PARAMETER.
	echo "0"
}

select_model() {
	if [[ -n "${GO_CHECK_IT_LOCAL_MODEL:-}" ]]; then
		echo "$GO_CHECK_IT_LOCAL_MODEL"
		return 0
	fi
	local models=()
	local line
	while IFS= read -r line; do
		[[ -n "$line" ]] || continue
		models+=("$line")
	done < <(list_models)
	if [[ ${#models[@]} -eq 0 ]]; then
		echo "No Ollama models are installed. Pull one first (e.g. ollama pull ...)." >&2
		exit 1
	fi
	if [[ ! -t 0 ]]; then
		echo "No TTY for interactive selection. Set GO_CHECK_IT_LOCAL_MODEL." >&2
		printf 'Installed models:\n' >&2
		printf '  %s\n' "${models[@]}" >&2
		exit 1
	fi
	echo "Installed Ollama models:" >&2
	local i=1
	for line in "${models[@]}"; do
		printf '  %2d) %s\n' "$i" "$line" >&2
		i=$((i + 1))
	done
	local choice
	read -r -p "Select model number for go-check-it-local: " choice
	if [[ ! "$choice" =~ ^[0-9]+$ ]] || ((choice < 1 || choice > ${#models[@]})); then
		echo "Invalid selection" >&2
		exit 1
	fi
	echo "${models[$((choice - 1))]}"
}

model_installed() {
	local want=$1
	local want_base=${want%%:*}
	local line base
	while IFS= read -r line; do
		[[ -z "$line" ]] && continue
		[[ "$line" == "$want" ]] && return 0
		base=${line%%:*}
		# Ollama often reports aliases as name:latest.
		if [[ "$base" == "$want_base" ]]; then
			return 0
		fi
		if [[ "$line" == "$want_base:latest" || "$want" == "$base:latest" ]]; then
			return 0
		fi
	done < <(list_models)
	return 1
}

create_alias() {
	local source_model=$1
	local tmp
	tmp=$(mktemp)
	cat >"$tmp" <<EOF
FROM ${source_model}
PARAMETER num_ctx ${MIN_CTX}
EOF
	ollama create "$ALIAS_NAME" -f "$tmp"
	rm -f -- "$tmp"
	echo "Created/updated Ollama alias ${ALIAS_NAME} <- ${source_model} (num_ctx=${MIN_CTX})"
}

check_alias() {
	if ! model_installed "$ALIAS_NAME"; then
		echo "Alias ${ALIAS_NAME} is not installed. Run: setup-opencode" >&2
		return 1
	fi
	echo "Alias ${ALIAS_NAME} is present"
	return 0
}

check_opencode() {
	if opencode_bin >/dev/null; then
		echo "OpenCode found at $(opencode_bin)"
		return 0
	fi
	echo "OpenCode is not on PATH" >&2
	echo "Install: https://opencode.ai/docs/  or run: setup-opencode --install" >&2
	return 1
}

case "$mode" in
	check)
		ensure_ollama
		check_opencode || true
		check_alias || exit 1
		echo "Preflight OK (non-mutating)"
		;;
	install)
		ensure_ollama
		echo "Launching Ollama's OpenCode integration (may offer to install OpenCode)..."
		echo "This does not install Cursor."
		exec ollama launch opencode
		;;
	setup)
		ensure_ollama
		check_opencode || true
		source_model=$(select_model)
		if ! model_installed "$source_model"; then
			echo "Model is not installed: ${source_model}" >&2
			echo "Install it with ollama pull, then rerun setup. This script never pulls models." >&2
			exit 1
		fi
		ctx=$(model_context_limit "$source_model")
		if [[ "$ctx" != "0" && "$ctx" -lt "$MIN_CTX" ]]; then
			echo "Model ${source_model} advertises context ${ctx}, below OpenCode's ${MIN_CTX} minimum." >&2
			echo "Choose a model with at least 64K context." >&2
			exit 1
		fi
		if [[ "$ctx" == "0" ]]; then
			echo "Could not verify advertised context for ${source_model}; creating alias with num_ctx=${MIN_CTX}." >&2
		fi
		create_alias "$source_model"
		echo "OpenCode provider model id: ollama/${ALIAS_NAME}"
		echo "Verify runtime allocation after first use with: ollama ps"
		echo "Start OpenCode with: ollama launch opencode   (or: opencode)"
		;;
esac
