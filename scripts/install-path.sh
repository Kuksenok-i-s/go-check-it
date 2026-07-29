#!/bin/sh
# install-path.sh — put go-check-it host tools on PATH and install OpenCode agents.
#
# Installs into ~/.local/bin (already on many PATH setups) and copies the
# local-* OpenCode agents into ~/.config/opencode/agents/ so
# run-local-subagent works from any project directory.

set -eu

force=0
bin_dir="${GO_CHECK_IT_BIN_DIR:-$HOME/.local/bin}"
agents_dir="${GO_CHECK_IT_OPENCODE_AGENTS_DIR:-$HOME/.config/opencode/agents}"
config_file="${GO_CHECK_IT_OPENCODE_CONFIG:-$HOME/.config/opencode/opencode.json}"

for arg in "$@"; do
	case "$arg" in
		--force) force=1 ;;
		-h | --help)
			cat <<'EOF'
usage: sh scripts/install-path.sh [--force]

	Installs:
    ~/.local/bin/run-local-subagent
    ~/.local/bin/run-local-swarm
    ~/.local/bin/run-small-subagent
    ~/.local/bin/run-go-check-it-agents
    ~/.local/bin/setup-opencode
    ~/.config/opencode/agents/local-*.md
    ~/.config/opencode/agents/small-*.md
  Merges the ollama/go-check-it-local model into ~/.config/opencode/opencode.json
  without removing existing providers/models.

Environment:
  GO_CHECK_IT_BIN_DIR               Override binary install dir (default ~/.local/bin)
  GO_CHECK_IT_OPENCODE_AGENTS_DIR   Override agents dir
  GO_CHECK_IT_OPENCODE_CONFIG       Override OpenCode config path
EOF
			exit 0
			;;
		*)
			echo "unknown option: $arg" >&2
			exit 2
			;;
	esac
done

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(dirname -- "$script_dir")

mkdir -p -- "$bin_dir" "$agents_dir" "$(dirname -- "$config_file")"

install_link() {
	src=$1
	dest=$2
	if [ -e "$dest" ] || [ -L "$dest" ]; then
		if [ "$force" -ne 1 ]; then
			# Replace only our managed symlinks; refuse to clobber unrelated files.
			if [ -L "$dest" ]; then
				target=$(readlink -- "$dest")
				case "$target" in
					"$repo_root"/scripts/*) ;;
					*)
						echo "refusing to overwrite $dest (not a go-check-it install); pass --force" >&2
						exit 1
						;;
				esac
			else
				echo "refusing to overwrite $dest (not a symlink); pass --force" >&2
				exit 1
			fi
		fi
		rm -f -- "$dest"
	fi
	ln -s -- "$src" "$dest"
	echo "installed $dest -> $src"
}

install_link "$script_dir/run-local-subagent.sh" "$bin_dir/run-local-subagent"
install_link "$script_dir/run-local-swarm.py" "$bin_dir/run-local-swarm"
install_link "$script_dir/run-small-subagent.sh" "$bin_dir/run-small-subagent"
install_link "$script_dir/run-go-check-it-agents.py" "$bin_dir/run-go-check-it-agents"
install_link "$script_dir/setup-opencode.sh" "$bin_dir/setup-opencode"

for agent in \
	local-lint-diagnosis \
	local-go-test-designer \
	local-crap-refactor \
	local-patch-review \
	local-project-scout \
	small-quality-worker \
	small-go-check-it-orchestrator
do
	src="$repo_root/.opencode/agents/$agent.md"
	dest="$agents_dir/$agent.md"
	if [ ! -f "$src" ]; then
		echo "missing agent definition: $src" >&2
		exit 1
	fi
	if [ -e "$dest" ] || [ -L "$dest" ]; then
		if [ "$force" -ne 1 ]; then
			echo "agent already present at $dest (rerun with --force to replace)" >&2
			continue
		fi
		rm -f -- "$dest"
	fi
	cp -- "$src" "$dest"
	echo "installed $dest"
done

# Merge go-check-it-local into the user OpenCode config without wiping other models.
python3 - "$config_file" <<'PY'
import json
import pathlib
import sys

path = pathlib.Path(sys.argv[1])
if path.exists():
    cfg = json.loads(path.read_text())
else:
    cfg = {"$schema": "https://opencode.ai/config.json"}

provider = cfg.setdefault("provider", {})
ollama = provider.setdefault("ollama", {})
ollama.setdefault("npm", "@ai-sdk/openai-compatible")
ollama.setdefault("name", ollama.get("name") or "Ollama (local)")
options = ollama.setdefault("options", {})
options.setdefault("baseURL", "http://localhost:11434/v1")
models = ollama.setdefault("models", {})
models["go-check-it-local"] = {
    "name": "go-check-it-local (64K)",
    "limit": {"context": 65536, "output": 8192},
}

path.write_text(json.dumps(cfg, indent=2) + "\n")
print(f"merged go-check-it-local into {path}")
PY

case ":$PATH:" in
	*":$bin_dir:"*) ;;
	*)
		echo "note: $bin_dir is not on PATH; add it to use run-local-subagent, run-local-swarm, run-small-subagent, run-go-check-it-agents, and setup-opencode" >&2
		;;
esac

echo "PATH tools ready. Create the Ollama alias with: setup-opencode"
echo "Verify: command -v run-local-subagent && run-local-subagent --help"
echo "Verify: command -v run-local-swarm && run-local-swarm --help"
echo "Verify: command -v run-small-subagent && run-small-subagent --help"
echo "Verify: command -v run-go-check-it-agents && run-go-check-it-agents --help"
echo "Small-model pipeline requires: export GO_CHECK_IT_SMALL_MODEL=provider/model"
