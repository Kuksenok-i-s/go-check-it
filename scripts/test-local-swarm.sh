#!/usr/bin/env bash
# Fake-based validation for run-local-swarm.py.
# Does not talk to real Ollama or OpenCode.

set -euo pipefail

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(dirname -- "$script_dir")
tmpdir=$(mktemp -d)
trap 'rm -rf -- "$tmpdir"' EXIT

fake_bin="$tmpdir/bin"
mkdir -p "$fake_bin" "$tmpdir/state"

# Fake leaf bridge: records args, concurrency, and can fail/sleep by role/prompt.
cat >"$fake_bin/run-local-subagent" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
state="${FAKE_STATE:?}"
mkdir -p "$state/starts" "$state/ends" "$state/inflight"

role=$1
shift
prompt=""
files=()
while [[ $# -gt 0 ]]; do
	case "$1" in
		--file)
			files+=("$2")
			shift 2
			;;
		--)
			shift
			prompt="$*"
			break
			;;
		*)
			prompt="$*"
			break
			;;
	esac
done

# Record invocation (one line JSON-ish for tests).
{
	printf 'role=%s\n' "$role"
	printf 'prompt=%s\n' "$prompt"
	for f in "${files[@]+"${files[@]}"}"; do
		printf 'file=%s\n' "$f"
	done
} >"$state/last-${role}.args"
printf '%s\n' "$*" >>"$state/all-invocations.log"

# Track concurrency with flock-like mkdir slots.
slot=
for i in 1 2 3 4 5 6 7 8; do
	if mkdir "$state/inflight/$i" 2>/dev/null; then
		slot=$i
		break
	fi
done
[[ -n "$slot" ]] || { echo "no inflight slot" >&2; exit 99; }

active=$(find "$state/inflight" -mindepth 1 -maxdepth 1 -type d | wc -l)
echo "$active" >>"$state/concurrency.log"
date +%s%N >"$state/starts/${role}.$$"

# Behavior knobs via prompt markers.
if [[ "$prompt" == *SLEEP* ]]; then
	sleep 2
fi
if [[ "$prompt" == *FAIL* ]]; then
	echo "fake failure: $role" >&2
	rmdir "$state/inflight/$slot" 2>/dev/null || true
	exit 7
fi
if [[ "$prompt" == *HANG* ]]; then
	sleep 30
fi

echo "{\"role\":\"$role\",\"ok\":true}"
date +%s%N >"$state/ends/${role}.$$"
rmdir "$state/inflight/$slot" 2>/dev/null || true
exit 0
EOF
chmod +x "$fake_bin/run-local-subagent"

export PATH="$fake_bin:$PATH"
export FAKE_STATE="$tmpdir/state"
export GO_CHECK_IT_SUBAGENT_BIN="$fake_bin/run-local-subagent"

swarm="$repo_root/scripts/run-local-swarm.py"
ctx="$tmpdir/ctx.txt"
echo 'context' >"$ctx"

fail() {
	echo "FAIL: $*" >&2
	exit 1
}

# --- usage: missing manifest ---
if python3 "$swarm" >/dev/null 2>&1; then
	fail "expected usage error without --manifest"
fi

# --- invalid role rejected ---
cat >"$tmpdir/bad-role.json" <<EOF
[{"id":"t1","role":"evil-agent","prompt":"nope"}]
EOF
if python3 "$swarm" --manifest "$tmpdir/bad-role.json" >/dev/null 2>&1; then
	fail "expected refusal for unknown role"
fi

# --- missing file rejected ---
cat >"$tmpdir/bad-file.json" <<EOF
[{"id":"t1","role":"local-project-scout","prompt":"x","files":["$tmpdir/missing.txt"]}]
EOF
if python3 "$swarm" --manifest "$tmpdir/bad-file.json" >/dev/null 2>&1; then
	fail "expected missing file error"
fi

# --- duplicate id rejected ---
cat >"$tmpdir/dup.json" <<EOF
[
  {"id":"t1","role":"local-project-scout","prompt":"a"},
  {"id":"t1","role":"local-project-scout","prompt":"b"}
]
EOF
if python3 "$swarm" --manifest "$tmpdir/dup.json" >/dev/null 2>&1; then
	fail "expected duplicate id error"
fi

# --- hard max workers ---
cat >"$tmpdir/ok.json" <<EOF
[{"id":"t1","role":"local-project-scout","prompt":"hello"}]
EOF
if python3 "$swarm" --manifest "$tmpdir/ok.json" --max-workers 5 >/dev/null 2>&1; then
	fail "expected hard max workers error"
fi

# --- happy path: ordered results, no --auto ---
rm -f "$FAKE_STATE"/*
mkdir -p "$FAKE_STATE"
cat >"$tmpdir/two.json" <<EOF
[
  {"id":"alpha","role":"local-project-scout","prompt":"scout A","files":["$ctx"]},
  {"id":"beta","role":"local-lint-diagnosis","prompt":"scout B"}
]
EOF
out="$tmpdir/out-ok.json"
python3 "$swarm" --manifest "$tmpdir/two.json" --max-workers 2 >"$out"
python3 - <<PY
import json
from pathlib import Path
env=json.loads(Path("$out").read_text())
assert env["ok"] is True
assert env["status"] == "ok"
assert [t["id"] for t in env["tasks"]] == ["alpha", "beta"]
assert all(t["status"] == "ok" for t in env["tasks"])
assert "local-project-scout" in env["tasks"][0]["stdout"]
assert "--auto" not in Path("$FAKE_STATE/all-invocations.log").read_text()
PY

# --- partial failure does not drop successes ---
rm -rf "$FAKE_STATE"/*
mkdir -p "$FAKE_STATE"
cat >"$tmpdir/partial.json" <<EOF
[
  {"id":"ok1","role":"local-project-scout","prompt":"good"},
  {"id":"bad","role":"local-crap-refactor","prompt":"please FAIL now"},
  {"id":"ok2","role":"local-go-test-designer","prompt":"also good"}
]
EOF
set +e
python3 "$swarm" --manifest "$tmpdir/partial.json" --max-workers 2 >"$tmpdir/out-partial.json"
rc=$?
set -e
[[ "$rc" -eq 1 ]] || fail "expected exit 1 for partial, got $rc"
python3 - <<PY
import json
from pathlib import Path
env=json.loads(Path("$tmpdir/out-partial.json").read_text())
assert env["ok"] is False
assert env["status"] == "partial"
by={t["id"]: t for t in env["tasks"]}
assert by["ok1"]["status"] == "ok"
assert by["bad"]["status"] == "error"
assert by["ok2"]["status"] == "ok"
assert [t["id"] for t in env["tasks"]] == ["ok1", "bad", "ok2"]
PY

# --- worker cap: never more than max-workers concurrent ---
rm -rf "$FAKE_STATE"/*
mkdir -p "$FAKE_STATE"
cat >"$tmpdir/conc.json" <<EOF
[
  {"id":"c1","role":"local-project-scout","prompt":"SLEEP one"},
  {"id":"c2","role":"local-project-scout","prompt":"SLEEP two"},
  {"id":"c3","role":"local-project-scout","prompt":"SLEEP three"},
  {"id":"c4","role":"local-project-scout","prompt":"SLEEP four"}
]
EOF
python3 "$swarm" --manifest "$tmpdir/conc.json" --max-workers 2 --task-timeout 10 --total-timeout 60 >"$tmpdir/out-conc.json"
python3 - <<PY
from pathlib import Path
lines=Path("$FAKE_STATE/concurrency.log").read_text().strip().splitlines()
assert lines, "no concurrency samples"
vals=[int(x) for x in lines]
assert max(vals) <= 2, f"concurrency exceeded: {vals}"
PY

# --- per-task timeout ---
rm -rf "$FAKE_STATE"/*
mkdir -p "$FAKE_STATE"
cat >"$tmpdir/timeout.json" <<EOF
[
  {"id":"hang","role":"local-project-scout","prompt":"please HANG"},
  {"id":"fast","role":"local-lint-diagnosis","prompt":"quick"}
]
EOF
set +e
python3 "$swarm" --manifest "$tmpdir/timeout.json" --max-workers 2 --task-timeout 1 --total-timeout 30 >"$tmpdir/out-timeout.json"
rc=$?
set -e
[[ "$rc" -eq 1 ]] || fail "expected exit 1 for timeout, got $rc"
python3 - <<PY
import json
from pathlib import Path
env=json.loads(Path("$tmpdir/out-timeout.json").read_text())
by={t["id"]: t for t in env["tasks"]}
assert by["hang"]["status"] == "timeout"
assert by["hang"]["timed_out"] is True
assert by["fast"]["status"] == "ok"
PY

# --- install-path installs swarm symlink (isolated dirs) ---
install_bin="$tmpdir/install-bin"
install_agents="$tmpdir/install-agents"
install_cfg="$tmpdir/install-opencode.json"
mkdir -p "$install_bin" "$install_agents"
GO_CHECK_IT_BIN_DIR="$install_bin" \
	GO_CHECK_IT_OPENCODE_AGENTS_DIR="$install_agents" \
	GO_CHECK_IT_OPENCODE_CONFIG="$install_cfg" \
	sh "$repo_root/scripts/install-path.sh" >/dev/null
[[ -L "$install_bin/run-local-swarm" ]] || fail "run-local-swarm symlink missing"
[[ -f "$install_agents/local-project-scout.md" ]] || fail "scout agent not installed"
target=$(readlink -- "$install_bin/run-local-swarm")
[[ "$target" == "$repo_root/scripts/run-local-swarm.py" ]] || fail "unexpected swarm symlink target: $target"

echo "local-swarm fake validation passed"
