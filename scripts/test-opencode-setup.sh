#!/usr/bin/env bash
# Fake-based validation for OpenCode/Ollama setup and the cross-IDE bridge.
# Creates no real Ollama models and does not launch OpenCode.

set -euo pipefail

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(dirname -- "$script_dir")
tmpdir=$(mktemp -d)
trap 'rm -rf -- "$tmpdir"' EXIT

fake_bin="$tmpdir/bin"
mkdir -p "$fake_bin"

cat >"$fake_bin/curl" <<'EOF'
#!/usr/bin/env bash
# Only the tags probe is required by these scripts.
if [[ "$*" == *"/api/tags"* ]]; then
	printf '%s\n' '{"models":[{"name":"demo-model:latest"},{"name":"other-chat:7b"},{"name":"nomic-embed-text:latest"},{"name":"go-check-it-local:latest"}]}'
	exit 0
fi
echo "unexpected curl invocation: $*" >&2
exit 1
EOF
chmod +x "$fake_bin/curl"

cat >"$fake_bin/ollama" <<'EOF'
#!/usr/bin/env bash
case "$1" in
	list)
		printf '%s\n' 'NAME                   ID    SIZE'
		printf '%s\n' 'demo-model:latest      x     1GB'
		printf '%s\n' 'go-check-it-local      y     1GB'
		;;
	show)
		printf '%s\n' 'Model'
		printf '%s\n' '  context length    131072'
		;;
	create)
		# Record create args; accept -f Modelfile
		printf '%s\n' "$*" >"${FAKE_STATE}/ollama-create.args"
		modelfile=
		while [[ $# -gt 0 ]]; do
			if [[ "$1" == "-f" ]]; then
				modelfile=$2
				break
			fi
			shift
		done
		cp -- "$modelfile" "${FAKE_STATE}/Modelfile"
		echo "created"
		;;
	launch)
		printf '%s\n' "$*" >"${FAKE_STATE}/ollama-launch.args"
		echo "launched"
		;;
	*)
		echo "unexpected ollama: $*" >&2
		exit 1
		;;
esac
EOF
chmod +x "$fake_bin/ollama"

cat >"$fake_bin/opencode" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$@" >"${FAKE_STATE}/opencode.args"
echo '{"ok":true}'
EOF
chmod +x "$fake_bin/opencode"

cat >"$fake_bin/python3" <<'EOF'
#!/usr/bin/env bash
exec /usr/bin/python3 "$@"
EOF
chmod +x "$fake_bin/python3"

export PATH="$fake_bin:$PATH"
export FAKE_STATE="$tmpdir/state"
mkdir -p "$FAKE_STATE"

# --- setup creates alias Modelfile with num_ctx ---
rm -f "$FAKE_STATE"/*
GO_CHECK_IT_LOCAL_MODEL=demo-model:latest bash "$repo_root/scripts/setup-opencode.sh" >/dev/null
grep -q 'FROM demo-model:latest' "$FAKE_STATE/Modelfile"
grep -q 'num_ctx 65536' "$FAKE_STATE/Modelfile"
grep -q 'create go-check-it-local' "$FAKE_STATE/ollama-create.args"

# --- auto-recommend + confirm accepts without TTY ---
rm -f "$FAKE_STATE"/*
unset GO_CHECK_IT_LOCAL_MODEL
GO_CHECK_IT_CONFIRM=1 bash "$repo_root/scripts/setup-opencode.sh" >"$FAKE_STATE/setup.out" 2>"$FAKE_STATE/setup.err"
grep -q 'FROM other-chat:7b' "$FAKE_STATE/Modelfile"
grep -q 'Recommended local model: other-chat:7b' "$FAKE_STATE/setup.err"
grep -q 'GO_CHECK_IT_CONFIRM set; accepting recommendation.' "$FAKE_STATE/setup.err"
# Embedding models and the alias itself must not be recommended.
grep -qv 'nomic-embed-text' "$FAKE_STATE/Modelfile"
grep -qv 'go-check-it-local' "$FAKE_STATE/Modelfile" || true
# FROM line should not be the alias
grep -q 'FROM other-chat:7b' "$FAKE_STATE/Modelfile"

# --- without confirm or pin, non-TTY setup refuses to mutate ---
rm -f "$FAKE_STATE/ollama-create.args" "$FAKE_STATE/Modelfile"
unset GO_CHECK_IT_LOCAL_MODEL GO_CHECK_IT_CONFIRM
if bash "$repo_root/scripts/setup-opencode.sh" >/dev/null 2>"$FAKE_STATE/noconfirm.err"; then
	echo "expected non-TTY setup without confirm to fail" >&2
	exit 1
fi
grep -q 'GO_CHECK_IT_CONFIRM=1' "$FAKE_STATE/noconfirm.err"
[[ ! -f "$FAKE_STATE/ollama-create.args" ]]

# --- check mode is non-mutating for create ---
rm -f "$FAKE_STATE/ollama-create.args" "$FAKE_STATE/Modelfile"
bash "$repo_root/scripts/setup-opencode.sh" --check >/dev/null
[[ ! -f "$FAKE_STATE/ollama-create.args" ]]

# --- bridge allowlists roles and pins model ---
rm -f "$FAKE_STATE/opencode.args"
ctx="$tmpdir/context.txt"
echo 'diag' >"$ctx"
bash "$repo_root/scripts/run-local-subagent.sh" local-lint-diagnosis --file "$ctx" -- "fix it" >/dev/null
mapfile -t args <"$FAKE_STATE/opencode.args"
# Expect: run --agent local-lint-diagnosis --format json --file ... --model ollama/go-check-it-local fix it
joined=$(printf '%s ' "${args[@]}")
[[ "$joined" == *"run --agent local-lint-diagnosis --format json --file ${ctx} --model ollama/go-check-it-local fix it "* ]] || {
	echo "unexpected opencode args: $joined" >&2
	exit 1
}
# Must not pass --auto
[[ "$joined" != *"--auto"* ]]

# --- unknown role refused ---
if bash "$repo_root/scripts/run-local-subagent.sh" evil-agent -- "nope" >/dev/null 2>&1; then
	echo "expected refusal for unknown role" >&2
	exit 1
fi

# --- opencode.json is valid JSON and omits default model ---
python3 - <<PY
import json
from pathlib import Path
cfg=json.loads(Path("$repo_root/opencode.json").read_text())
assert "model" not in cfg, "primary model must remain unset"
assert "go-check-it-local" in cfg["provider"]["ollama"]["models"]
assert cfg["provider"]["ollama"]["models"]["go-check-it-local"]["limit"]["context"] == 65536
agents=cfg["agent"]["build"]["permission"]["task"]
for role in ("local-lint-diagnosis","local-go-test-designer","local-crap-refactor","local-patch-review","local-project-scout"):
    assert agents[role] == "allow"
assert agents["*"] == "ask"
PY

for agent in local-lint-diagnosis local-go-test-designer local-crap-refactor local-patch-review local-project-scout; do
	test -f "$repo_root/.opencode/agents/${agent}.md"
done

# --- scout role is allowlisted by the bridge ---
rm -f "$FAKE_STATE/opencode.args"
bash "$repo_root/scripts/run-local-subagent.sh" local-project-scout -- "scout entry points" >/dev/null
joined=$(tr '\n' ' ' <"$FAKE_STATE/opencode.args")
[[ "$joined" == *"run --agent local-project-scout --format json --model ollama/go-check-it-local scout entry points "* ]]
[[ "$joined" != *"--auto"* ]]

echo "opencode fake validation passed"
