#!/bin/sh

set -eu

usage() {
	echo "usage: sh scripts/install-agent-skill.sh <cursor|vscode|claude|codex> [--force]" >&2
	exit 2
}

[ "$#" -ge 1 ] && [ "$#" -le 2 ] || usage

platform=$1
force=${2:-}
[ -z "$force" ] || [ "$force" = "--force" ] || usage

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(dirname -- "$script_dir")
source_dir="$repo_root/.agents/skills/go-check-it"

case "$platform" in
	cursor) base_dir="$HOME/.cursor/skills" ;;
	vscode) base_dir="$HOME/.copilot/skills" ;;
	claude) base_dir="$HOME/.claude/skills" ;;
	codex) base_dir="$HOME/.agents/skills" ;;
	*) usage ;;
esac

destination="$base_dir/go-check-it"
if [ -e "$destination" ] || [ -L "$destination" ]; then
	if [ "$force" != "--force" ]; then
		echo "go-check-it is already installed at $destination" >&2
		echo "rerun with --force to replace it" >&2
		exit 1
	fi
	rm -rf -- "$destination"
fi

mkdir -p -- "$base_dir"
cp -R -- "$source_dir" "$destination"
echo "installed go-check-it for $platform at $destination"
