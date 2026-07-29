#!/usr/bin/env bash
# Fake-based validation for run-go-check-it-agents.py and run-small-subagent.sh.
# Does not talk to real OpenCode or cloud providers.

set -euo pipefail

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(dirname -- "$script_dir")
tmpdir=$(mktemp -d)
trap 'rm -rf -- "$tmpdir"' EXIT

fake_bin="$tmpdir/bin"
mkdir -p "$fake_bin" "$tmpdir/state"

cat >"$fake_bin/opencode" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
state="${FAKE_STATE:?}"
mkdir -p "$state"
printf '%s\n' "$*" >>"$state/opencode-args.log"

# Refuse --auto
for a in "$@"; do
	if [[ "$a" == "--auto" ]]; then
		echo "unexpected --auto" >&2
		exit 99
	fi
done

agent=""
model=""
prompt=""
while [[ $# -gt 0 ]]; do
	case "$1" in
		run)
			shift
			;;
		--agent)
			agent=$2
			shift 2
			;;
		--model)
			model=$2
			shift 2
			;;
		--format | --file)
			shift 2
			;;
		--)
			shift
			prompt="$*"
			break
			;;
		-*)
			shift
			;;
		*)
			prompt="$*"
			break
			;;
	esac
done

echo "$agent|$model" >>"$state/invocations.log"

case "$agent" in
	small-quality-worker)
		id=$(printf '%s' "$prompt" | sed -n 's/.*task_id=\([^ .]*\).*/\1/p')
		[[ -n "$id" ]] || id="cluster-1"
		if [[ "$prompt" == *FAIL* ]]; then
			echo "worker boom" >&2
			exit 7
		fi
		if [[ "$prompt" == *BADJSON* ]]; then
			echo "not json at all"
			exit 0
		fi
		if [[ "$prompt" == *RACE* ]]; then
			cat <<JSON
{"task_id":"$id","status":"escalate","diagnosis":"data race in cache","evidence":[{"file":"cache.go","line":33,"note":"write"}],"proposed_diff":"","focused_commands":["go test -race ./internal/cache/..."],"confidence":4,"escalate_reasons":["race"],"unresolved_risks":[]}
JSON
			exit 0
		fi
		cat <<JSON
{"task_id":"$id","status":"ok","diagnosis":"extract helper","evidence":[{"file":"x.go","line":1,"note":"high CRAP"}],"proposed_diff":"diff --git a/x.go b/x.go\n","focused_commands":["go test ./pkg/..."],"confidence":8,"escalate_reasons":[],"unresolved_risks":[]}
JSON
		;;
	small-go-check-it-orchestrator)
		if [[ "$prompt" == *ORCHFAIL* ]]; then
			echo "orchestrator down" >&2
			exit 3
		fi
		cat <<'JSON'
{"ok":true,"status":"ready","summary":"Apply one helper extraction.","recommendations":[{"id":"cluster-1","action":"apply_diff","diagnosis":"extract helper","evidence":[{"file":"x.go","line":1,"note":"high CRAP"}],"proposed_diff":"diff --git a/x.go b/x.go\n","focused_commands":["go test ./pkg/..."],"confidence":8,"worker_ids":["cluster-1"]}],"disagreements":[],"unresolved_risks":[],"escalate_reasons":[],"omitted":{"hotspots":0,"findings":0},"raw_logs_dir":""}
JSON
		;;
	*)
		echo "unknown agent $agent" >&2
		exit 2
		;;
esac
EOF
chmod +x "$fake_bin/opencode"

export PATH="$fake_bin:$PATH"
export FAKE_STATE="$tmpdir/state"
export OPENCODE_BIN="$fake_bin/opencode"
export GO_CHECK_IT_SMALL_MODEL="anthropic/claude-haiku-fake"
export GO_CHECK_IT_SMALL_SUBAGENT_BIN="$repo_root/scripts/run-small-subagent.sh"

pipeline="$repo_root/scripts/run-go-check-it-agents.py"
fail() { echo "FAIL: $*" >&2; exit 1; }

# --- small-subagent refuses without model ---
unset GO_CHECK_IT_SMALL_MODEL
if bash "$repo_root/scripts/run-small-subagent.sh" small-quality-worker -- "x" >/dev/null 2>&1; then
	fail "expected missing model error"
fi
export GO_CHECK_IT_SMALL_MODEL="anthropic/claude-haiku-fake"

# --- small-subagent refuses unknown role ---
if bash "$repo_root/scripts/run-small-subagent.sh" evil-agent -- "x" >/dev/null 2>&1; then
	fail "expected unknown role refusal"
fi

# --- small-subagent pins model and omits --auto ---
rm -f "$FAKE_STATE"/*
mkdir -p "$FAKE_STATE"
bash "$repo_root/scripts/run-small-subagent.sh" small-quality-worker -- "task_id=cluster-1 hello" >/dev/null
grep -q 'small-quality-worker|anthropic/claude-haiku-fake' "$FAKE_STATE/invocations.log" || fail "model not pinned"
grep -vq -- '--auto' "$FAKE_STATE/opencode-args.log" || fail "unexpected --auto"

# --- agent-json fixture ---
cat >"$tmpdir/agent.json" <<'EOF'
{
  "summary": {
    "threshold": 8,
    "maxCrap": 30.5,
    "maxCrapFunction": "Risky",
    "maxCrapFile": "pkg/a/a.go",
    "maxCrapLine": 10,
    "functionCount": 20,
    "aboveThresholdCount": 3,
    "findingCount": 0,
    "hotspotLimit": 6
  },
  "hotspots": [
    {"functionName":"Risky","package":"a","file":"pkg/a/a.go","line":10,"complexity":8,"coveragePercent":10,"crapScore":30.5},
    {"functionName":"Other","package":"a","file":"pkg/a/a.go","line":40,"complexity":6,"coveragePercent":20,"crapScore":12},
    {"functionName":"B1","package":"b","file":"pkg/b/b.go","line":5,"complexity":5,"coveragePercent":15,"crapScore":11},
    {"functionName":"C1","package":"c","file":"pkg/c/c.go","line":5,"complexity":5,"coveragePercent":15,"crapScore":10},
    {"functionName":"D1","package":"d","file":"pkg/d/d.go","line":5,"complexity":5,"coveragePercent":15,"crapScore":9.5},
    {"functionName":"E1","package":"e","file":"pkg/e/e.go","line":5,"complexity":5,"coveragePercent":15,"crapScore":9.2},
    {"functionName":"F1","package":"f","file":"pkg/f/f.go","line":5,"complexity":5,"coveragePercent":15,"crapScore":9.1},
    {"functionName":"G1","package":"g","file":"pkg/g/g.go","line":5,"complexity":5,"coveragePercent":15,"crapScore":9.0}
  ],
  "findings": [],
  "omitted": {"functionsAboveHotspots": 0, "functionsTotalOmitted": 12}
}
EOF

# --- happy path with orchestrator ---
rm -rf "$FAKE_STATE"/*
mkdir -p "$FAKE_STATE"
out="$tmpdir/out-ok.json"
python3 "$pipeline" --agent-json "$tmpdir/agent.json" --max-workers 2 >"$out"
python3 - <<PY
import json
from pathlib import Path
env=json.loads(Path("$out").read_text())
assert env["status"] in {"ready", "partial", "escalate"}
assert "recommendations" in env
assert "tasks" not in env  # no raw swarm dump
assert "WARNING: DATA RACE" not in json.dumps(env)
# clustering: 8 packages but max 6 clusters
inv=Path("$FAKE_STATE/invocations.log").read_text().strip().splitlines()
workers=[l for l in inv if l.startswith("small-quality-worker|")]
assert len(workers) <= 6, workers
assert any(l.startswith("small-go-check-it-orchestrator|") for l in inv), inv
# raw logs dir exists and is private path referenced
assert env.get("raw_logs_dir"), env
assert Path(env["raw_logs_dir"]).is_dir()
assert (Path(env["raw_logs_dir"]) / "raw-workers.json").is_file()
# normalized workers exclude full stdout
norm=json.loads((Path(env["raw_logs_dir"]) / "normalized-workers.json").read_text())
assert all("stdout" not in w for w in norm)
PY

# --- hard max workers ---
if python3 "$pipeline" --agent-json "$tmpdir/agent.json" --max-workers 7 >/dev/null 2>&1; then
	fail "expected hard max workers error"
fi

# --- race escalation via worker ---
cat >"$tmpdir/race-agent.json" <<'EOF'
{
  "summary": {"threshold": 8, "maxCrap": 9, "functionCount": 1, "aboveThresholdCount": 1, "findingCount": 0, "hotspotLimit": 6},
  "hotspots": [{"functionName":"Set","package":"cache","file":"cache.go","line":33,"complexity":3,"crapScore":9}],
  "findings": [],
  "omitted": {"functionsAboveHotspots": 0, "functionsTotalOmitted": 0}
}
EOF
rm -rf "$FAKE_STATE"/*
mkdir -p "$FAKE_STATE"
# Force worker prompt marker by patching cluster file after first stage is hard;
# instead call small worker directly with RACE marker through pipeline by
# putting RACE in function name used in prompt.
python3 - <<'PY' >"$tmpdir/race-agent.json"
import json
print(json.dumps({
  "summary": {"threshold": 8, "maxCrap": 9, "functionCount": 1, "aboveThresholdCount": 1, "findingCount": 0, "hotspotLimit": 6},
  "hotspots": [{"functionName":"RACESet","package":"cache","file":"cache.go","line":33,"complexity":3,"crapScore":9}],
  "findings": [],
  "omitted": {"functionsAboveHotspots": 0, "functionsTotalOmitted": 0}
}))
PY
# Fake opencode matches *RACE* in prompt (function names are included).
set +e
python3 "$pipeline" --agent-json "$tmpdir/race-agent.json" --max-workers 1 --skip-orchestrator >"$tmpdir/out-race.json"
rc=$?
set -e
[[ "$rc" -eq 1 ]] || fail "expected partial/escalate exit for race, got $rc"
python3 - <<PY
import json
from pathlib import Path
env=json.loads(Path("$tmpdir/out-race.json").read_text())
assert env["status"] in {"escalate", "partial"}
assert env.get("escalate_reasons") or any(r.get("action")=="manual_review" for r in env["recommendations"])
PY

# --- bad worker JSON falls back without dumping transcript ---
python3 - <<'PY' >"$tmpdir/bad-agent.json"
import json
print(json.dumps({
  "summary": {"threshold": 8, "maxCrap": 9, "functionCount": 1, "aboveThresholdCount": 1, "findingCount": 0, "hotspotLimit": 6},
  "hotspots": [{"functionName":"BADJSONFn","package":"p","file":"p.go","line":1,"complexity":3,"crapScore":9}],
  "findings": [],
  "omitted": {"functionsAboveHotspots": 0, "functionsTotalOmitted": 0}
}))
PY
rm -rf "$FAKE_STATE"/*
mkdir -p "$FAKE_STATE"
python3 "$pipeline" --agent-json "$tmpdir/bad-agent.json" --skip-orchestrator >"$tmpdir/out-bad.json" || true
python3 - <<PY
import json
from pathlib import Path
env=json.loads(Path("$tmpdir/out-bad.json").read_text())
assert "recommendations" in env
text=json.dumps(env)
assert "not json at all" not in text
PY

# --- install-path installs new tools ---
install_bin="$tmpdir/install-bin"
install_agents="$tmpdir/install-agents"
install_cfg="$tmpdir/install-opencode.json"
mkdir -p "$install_bin" "$install_agents"
GO_CHECK_IT_BIN_DIR="$install_bin" \
	GO_CHECK_IT_OPENCODE_AGENTS_DIR="$install_agents" \
	GO_CHECK_IT_OPENCODE_CONFIG="$install_cfg" \
	sh "$repo_root/scripts/install-path.sh" >/dev/null
[[ -L "$install_bin/run-small-subagent" ]] || fail "run-small-subagent missing"
[[ -L "$install_bin/run-go-check-it-agents" ]] || fail "run-go-check-it-agents missing"
[[ -f "$install_agents/small-quality-worker.md" ]] || fail "small-quality-worker agent missing"
[[ -f "$install_agents/small-go-check-it-orchestrator.md" ]] || fail "orchestrator agent missing"

echo "go-check-it-agents fake validation passed"
