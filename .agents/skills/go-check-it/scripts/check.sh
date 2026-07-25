#!/usr/bin/env bash

set -euo pipefail

if repo_root=$(git rev-parse --show-toplevel 2>/dev/null); then
	cd "$repo_root"
fi

unformatted=$(gofmt -l .)
if [[ -n "$unformatted" ]]; then
	printf 'gofmt required:\n%s\n' "$unformatted" >&2
	exit 1
fi

go test ./...
go test -race ./...
go vet ./...

if ! command -v golangci-lint >/dev/null 2>&1; then
	echo "golangci-lint is required but was not found on PATH" >&2
	echo "install it from https://golangci-lint.run/docs/welcome/install/" >&2
	exit 1
fi
golangci-lint run ./...

tmpdir=
cleanup() {
	if [[ -n "$tmpdir" ]]; then
		rm -rf -- "$tmpdir"
	fi
}
trap cleanup EXIT

# Prefer a local build only inside the go-check-it source repo (dogfood).
# Everywhere else, use the installed binary. Never treat a vendored copy in a
# consumer repo as intentional.
is_go_check_it_source=0
if [[ -f go.mod ]] && grep -qx 'module go-check-it' go.mod; then
	is_go_check_it_source=1
fi

if [[ "$is_go_check_it_source" -eq 1 && -d cmd/go-check-it ]]; then
	tmpdir=$(mktemp -d)
	go build -o "$tmpdir/go-check-it" ./cmd/go-check-it
	go_check_it_bin="$tmpdir/go-check-it"
elif command -v go-check-it >/dev/null 2>&1; then
	go_check_it_bin=$(command -v go-check-it)
elif [[ -d cmd/go-check-it ]]; then
	echo "found cmd/go-check-it outside the go-check-it source repository" >&2
	echo "remove that tree and install go-check-it globally instead" >&2
	echo "see references/INSTALL.md" >&2
	exit 1
else
	echo "go-check-it is required but was not found on PATH" >&2
	echo "install it as described in references/INSTALL.md" >&2
	echo "do not copy or port go-check-it into this repository" >&2
	exit 1
fi

"$go_check_it_bin" --explain --format=json --fail-on-findings
