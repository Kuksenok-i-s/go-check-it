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

  (default)  Auto-recommend an installed Ollama model, ask for confirmation,
             then create/update go-check-it-local with a 64K context window.
  --check    Validate ollama, OpenCode, and the go-check-it-local alias only.
  --install  Run `ollama launch opencode` (may offer to install OpenCode).

Environment:
  GO_CHECK_IT_LOCAL_MODEL   Use this installed model (skips recommendation UI).
  GO_CHECK_IT_CONFIRM=1     Accept the auto-recommendation without a TTY prompt.
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

# Collect unique candidate models (skip the alias itself, embeddings, and
# bare duplicates when a tagged form exists).
candidate_models() {
	local line base other other_base
	local tagged=()
	local bare=()
	local out=()
	while IFS= read -r line; do
		[[ -n "$line" ]] || continue
		base=${line%%:*}
		[[ "$base" == "$ALIAS_NAME" || "$line" == "$ALIAS_NAME" ]] && continue
		case "$line" in
			*embed* | *Embed*) continue ;;
		esac
		if [[ "$line" == *:* ]]; then
			tagged+=("$line")
		else
			bare+=("$line")
		fi
	done < <(list_models)
	if [[ ${#tagged[@]} -gt 0 ]]; then
		for line in "${tagged[@]}"; do
			out+=("$line")
		done
	fi
	if [[ ${#bare[@]} -gt 0 ]]; then
		for line in "${bare[@]}"; do
			local has_tagged=0
			if [[ ${#tagged[@]} -gt 0 ]]; then
				for other in "${tagged[@]}"; do
					other_base=${other%%:*}
					if [[ "$other_base" == "$line" ]]; then
						has_tagged=1
						break
					fi
				done
			fi
			[[ "$has_tagged" -eq 1 ]] && continue
			out+=("$line")
		done
	fi
	if [[ ${#out[@]} -eq 0 ]]; then
		return 1
	fi
	printf '%s\n' "${out[@]}"
}

# Higher score is better. Prints: <score>\t<model>\t<reason>
score_model() {
	local model=$1
	local ctx=$2
	local score=0
	local reasons=()
	local lower
	lower=$(printf '%s' "$model" | tr '[:upper:]' '[:lower:]')

	if [[ "$ctx" != "0" && "$ctx" -lt "$MIN_CTX" ]]; then
		# Disqualify models that advertise below OpenCode's minimum.
		echo "-1000000	${model}	context ${ctx} < ${MIN_CTX}"
		return 0
	fi
	if [[ "$ctx" != "0" && "$ctx" -ge "$MIN_CTX" ]]; then
		score=$((score + 50))
		reasons+=("context ${ctx}")
	else
		score=$((score + 10))
		reasons+=("context unverified")
	fi
	case "$lower" in
		*coder*)
			score=$((score + 100))
			reasons+=("coder model")
			;;
		*code*)
			score=$((score + 60))
			reasons+=("code-oriented name")
			;;
	esac
	case "$lower" in
		qwen*|gpt-oss*|gemma*|llama*|mistral*|deepseek*)
			score=$((score + 15))
			;;
	esac
	# Prefer smaller tagged sizes when the tag encodes a parameter count.
	if [[ "$lower" =~ :([0-9]+(\.[0-9]+)?)b ]]; then
		local params=${BASH_REMATCH[1]}
		# Soft preference: ~7–14B for local specialists.
		if awk -v p="$params" 'BEGIN { exit !(p >= 7 && p <= 14) }'; then
			score=$((score + 25))
			reasons+=("${params}B size band")
		elif awk -v p="$params" 'BEGIN { exit !(p < 7) }'; then
			score=$((score + 5))
		else
			# Large models still usable; slight penalty vs mid-size.
			score=$((score - 5))
			reasons+=("large ${params}B")
		fi
	fi
	local reason
	if [[ ${#reasons[@]} -eq 0 ]]; then
		reason="installed candidate"
	else
		printf -v reason '%s, ' "${reasons[@]}"
		reason=${reason%, }
	fi
	printf '%s\t%s\t%s\n' "$score" "$model" "$reason"
}

recommend_model() {
	local models=()
	local line
	while IFS= read -r line; do
		[[ -n "$line" ]] || continue
		models+=("$line")
	done < <(candidate_models) || true
	if [[ ${#models[@]} -eq 0 ]]; then
		echo "No suitable Ollama models are installed. Pull a chat/code model first." >&2
		echo "Embedding-only models and the go-check-it-local alias are ignored." >&2
		exit 1
	fi

	local best_score=-999999999
	local best_model=""
	local best_reason=""
	local score model reason ctx ranked rest
	local -a ranking=()
	for model in "${models[@]}"; do
		ctx=$(model_context_limit "$model")
		ranked=$(score_model "$model" "$ctx")
		score=${ranked%%$'\t'*}
		rest=${ranked#*$'\t'}
		model=${rest%%$'\t'*}
		reason=${rest#*$'\t'}
		ranking+=("$ranked")
		if ((score > best_score)); then
			best_score=$score
			best_model=$model
			best_reason=$reason
		fi
	done
	if [[ -z "$best_model" || "$best_score" -le -1000000 ]]; then
		echo "No installed model meets OpenCode's ${MIN_CTX} context minimum." >&2
		printf 'Candidates:\n' >&2
		printf '  %s\n' "${models[@]}" >&2
		exit 1
	fi
	RECOMMENDED_MODEL=$best_model
	RECOMMENDED_REASON=$best_reason
	RECOMMENDED_ALTERNATES=()
	local entry alt_score alt_model
	for entry in "${ranking[@]}"; do
		alt_score=${entry%%$'\t'*}
		rest=${entry#*$'\t'}
		alt_model=${rest%%$'\t'*}
		[[ "$alt_model" == "$best_model" ]] && continue
		((alt_score > -1000000)) || continue
		RECOMMENDED_ALTERNATES+=("$alt_model")
	done
}

confirm_recommended_model() {
	local recommended=$1
	local reason=$2
	shift 2
	local alternates=("$@")

	echo "Recommended local model: ${recommended}" >&2
	echo "  reason: ${reason}" >&2
	if [[ ${#alternates[@]} -gt 0 ]]; then
		echo "Other installed candidates:" >&2
		local alt
		for alt in "${alternates[@]}"; do
			printf '  - %s\n' "$alt" >&2
		done
	fi

	if [[ "${GO_CHECK_IT_CONFIRM:-}" == "1" || "${GO_CHECK_IT_CONFIRM:-}" == "yes" ]]; then
		echo "GO_CHECK_IT_CONFIRM set; accepting recommendation." >&2
		echo "$recommended"
		return 0
	fi

	if [[ ! -t 0 ]]; then
		echo "Confirm the recommendation, then rerun:" >&2
		echo "  GO_CHECK_IT_CONFIRM=1 setup-opencode" >&2
		echo "Or pin an explicit model:" >&2
		echo "  GO_CHECK_IT_LOCAL_MODEL=${recommended} setup-opencode" >&2
		exit 1
	fi

	local reply
	read -r -p "Use this model for go-check-it-local? [Y/n/model-name] " reply
	if [[ -z "$reply" || "$reply" == "Y" || "$reply" == "y" || "$reply" == "yes" ]]; then
		echo "$recommended"
		return 0
	fi
	if [[ "$reply" == "n" || "$reply" == "N" || "$reply" == "no" ]]; then
		echo "Aborted. Re-run and accept, or set GO_CHECK_IT_LOCAL_MODEL=<model>." >&2
		exit 1
	fi
	# Treat any other input as an explicit installed model name.
	echo "$reply"
}

select_model() {
	if [[ -n "${GO_CHECK_IT_LOCAL_MODEL:-}" ]]; then
		echo "$GO_CHECK_IT_LOCAL_MODEL"
		return 0
	fi
	recommend_model
	if [[ ${#RECOMMENDED_ALTERNATES[@]} -gt 0 ]]; then
		confirm_recommended_model "$RECOMMENDED_MODEL" "$RECOMMENDED_REASON" "${RECOMMENDED_ALTERNATES[@]}"
	else
		confirm_recommended_model "$RECOMMENDED_MODEL" "$RECOMMENDED_REASON"
	fi
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
