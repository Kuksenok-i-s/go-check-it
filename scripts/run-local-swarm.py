#!/usr/bin/env python3
"""run-local-swarm.py — bounded fan-out over run-local-subagent.

Opt-in parallel launcher for allowlisted read-only OpenCode/Ollama roles.
The primary IDE agent synthesizes results; this script never edits files.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from pathlib import Path
from typing import Any

SCRIPT_DIR = Path(__file__).resolve().parent
sys.path.insert(0, str(SCRIPT_DIR))

from lib.swarm_runtime import SwarmTask, run_swarm  # noqa: E402

ALLOWED_ROLES = frozenset(
    {
        "local-lint-diagnosis",
        "local-go-test-designer",
        "local-crap-refactor",
        "local-patch-review",
        "local-project-scout",
    }
)

DEFAULT_MAX_WORKERS = 2
HARD_MAX_WORKERS = 4
MAX_TASKS = 8
DEFAULT_TASK_TIMEOUT = 120
DEFAULT_TOTAL_TIMEOUT = 300
MAX_CONTEXT_BYTES = 512 * 1024  # 512 KiB per attached file

EXIT_OK = 0
EXIT_PARTIAL = 1
EXIT_USAGE = 2


class SwarmError(Exception):
    """Invalid input or configuration."""


def script_dir() -> Path:
    return SCRIPT_DIR


def shutil_which(name: str) -> str | None:
    path = os.environ.get("PATH", "")
    for directory in path.split(os.pathsep):
        if not directory:
            continue
        candidate = Path(directory) / name
        if candidate.is_file() and os.access(candidate, os.X_OK):
            return str(candidate)
    return None


def resolve_bridge() -> Path:
    env = os.environ.get("GO_CHECK_IT_SUBAGENT_BIN")
    if env:
        path = Path(env)
        if path.is_file() and os.access(path, os.X_OK):
            return path
        raise SwarmError(f"GO_CHECK_IT_SUBAGENT_BIN is not executable: {env}")

    sibling = script_dir() / "run-local-subagent.sh"
    if sibling.is_file() and os.access(sibling, os.X_OK):
        return sibling

    which = shutil_which("run-local-subagent")
    if which:
        return Path(which)

    raise SwarmError(
        "run-local-subagent not found; install with: sh scripts/install-path.sh"
    )


def load_manifest(path: Path) -> list[SwarmTask]:
    try:
        raw = json.loads(path.read_text(encoding="utf-8"))
    except OSError as exc:
        raise SwarmError(f"cannot read manifest: {exc}") from exc
    except json.JSONDecodeError as exc:
        raise SwarmError(f"manifest is not valid JSON: {exc}") from exc

    if isinstance(raw, dict) and "tasks" in raw:
        items = raw["tasks"]
    else:
        items = raw

    if not isinstance(items, list):
        raise SwarmError("manifest must be a JSON array or an object with 'tasks'")
    if not items:
        raise SwarmError("manifest has no tasks")
    if len(items) > MAX_TASKS:
        raise SwarmError(f"manifest has {len(items)} tasks; maximum is {MAX_TASKS}")

    seen: set[str] = set()
    tasks: list[SwarmTask] = []
    for index, item in enumerate(items):
        if not isinstance(item, dict):
            raise SwarmError(f"task[{index}] must be an object")
        task_id = item.get("id")
        role = item.get("role")
        prompt = item.get("prompt", "")
        files = item.get("files") or []

        if not isinstance(task_id, str) or not task_id.strip():
            raise SwarmError(f"task[{index}] requires a non-empty string 'id'")
        if task_id in seen:
            raise SwarmError(f"duplicate task id: {task_id}")
        seen.add(task_id)

        if not isinstance(role, str) or role not in ALLOWED_ROLES:
            raise SwarmError(
                f"task[{index}] role must be one of: {', '.join(sorted(ALLOWED_ROLES))}"
            )
        if not isinstance(prompt, str):
            raise SwarmError(f"task[{index}] 'prompt' must be a string")
        if not isinstance(files, list) or not all(isinstance(f, str) for f in files):
            raise SwarmError(f"task[{index}] 'files' must be an array of strings")
        if not prompt.strip() and not files:
            raise SwarmError(f"task[{index}] requires a prompt or at least one file")

        resolved_files: list[str] = []
        for file_path in files:
            p = Path(file_path)
            if not p.is_file():
                raise SwarmError(f"task[{index}] context file not found: {file_path}")
            size = p.stat().st_size
            if size > MAX_CONTEXT_BYTES:
                raise SwarmError(
                    f"task[{index}] context file too large "
                    f"({size} bytes > {MAX_CONTEXT_BYTES}): {file_path}"
                )
            resolved_files.append(str(p.resolve()))

        tasks.append(
            SwarmTask(
                id=task_id.strip(),
                role=role,
                prompt=prompt.strip(),
                files=resolved_files,
            )
        )
    return tasks


def build_command(bridge: Path, task: SwarmTask) -> list[str]:
    cmd = [str(bridge), task.role]
    for file_path in task.files:
        cmd.extend(["--file", file_path])
    if task.prompt:
        cmd.extend(["--", task.prompt])
    return cmd


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        prog="run-local-swarm",
        description=(
            "Run up to 8 allowlisted read-only local subagents in parallel "
            "(default 2 workers, hard max 4)."
        ),
    )
    parser.add_argument(
        "--manifest",
        required=True,
        type=Path,
        help='JSON file: array of tasks or {"tasks": [...]}',
    )
    parser.add_argument(
        "--max-workers",
        type=int,
        default=DEFAULT_MAX_WORKERS,
        help=f"parallel tasks (default {DEFAULT_MAX_WORKERS}, hard max {HARD_MAX_WORKERS})",
    )
    parser.add_argument(
        "--task-timeout",
        type=float,
        default=DEFAULT_TASK_TIMEOUT,
        help=f"per-task timeout seconds (default {DEFAULT_TASK_TIMEOUT})",
    )
    parser.add_argument(
        "--total-timeout",
        type=float,
        default=DEFAULT_TOTAL_TIMEOUT,
        help=f"overall swarm timeout seconds (default {DEFAULT_TOTAL_TIMEOUT})",
    )
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)

    if args.max_workers < 1:
        print("error: --max-workers must be >= 1", file=sys.stderr)
        return EXIT_USAGE
    if args.max_workers > HARD_MAX_WORKERS:
        print(
            f"error: --max-workers hard max is {HARD_MAX_WORKERS}",
            file=sys.stderr,
        )
        return EXIT_USAGE
    if args.task_timeout <= 0 or args.total_timeout <= 0:
        print("error: timeouts must be positive", file=sys.stderr)
        return EXIT_USAGE

    try:
        tasks = load_manifest(args.manifest)
        bridge = resolve_bridge()
    except SwarmError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return EXIT_USAGE

    ordered, overall_status = run_swarm(
        tasks=tasks,
        max_workers=args.max_workers,
        task_timeout=args.task_timeout,
        total_timeout=args.total_timeout,
        build_command=lambda task: build_command(bridge, task),
    )

    envelope: dict[str, Any] = {
        "ok": overall_status == "ok",
        "status": overall_status,
        "max_workers": args.max_workers,
        "task_timeout_sec": args.task_timeout,
        "total_timeout_sec": args.total_timeout,
        "tasks": ordered,
    }
    json.dump(envelope, sys.stdout, indent=2, ensure_ascii=False)
    sys.stdout.write("\n")

    if overall_status == "ok":
        return EXIT_OK
    return EXIT_PARTIAL


if __name__ == "__main__":
    sys.exit(main())
