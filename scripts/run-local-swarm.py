#!/usr/bin/env python3
"""run-local-swarm.py — bounded fan-out over run-local-subagent.

Opt-in parallel launcher for allowlisted read-only OpenCode/Ollama roles.
The primary IDE agent synthesizes results; this script never edits files.
"""

from __future__ import annotations

import argparse
import json
import os
import signal
import subprocess
import sys
import threading
import time
from concurrent.futures import ThreadPoolExecutor, wait, FIRST_COMPLETED
from dataclasses import dataclass
from pathlib import Path
from typing import Any

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


@dataclass
class Task:
    id: str
    role: str
    prompt: str
    files: list[str]


class SwarmError(Exception):
    """Invalid input or configuration."""


def script_dir() -> Path:
    return Path(__file__).resolve().parent


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


def shutil_which(name: str) -> str | None:
    path = os.environ.get("PATH", "")
    for directory in path.split(os.pathsep):
        if not directory:
            continue
        candidate = Path(directory) / name
        if candidate.is_file() and os.access(candidate, os.X_OK):
            return str(candidate)
    return None


def load_manifest(path: Path) -> list[Task]:
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
    tasks: list[Task] = []
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
            Task(
                id=task_id.strip(),
                role=role,
                prompt=prompt.strip(),
                files=resolved_files,
            )
        )
    return tasks


def build_command(bridge: Path, task: Task) -> list[str]:
    cmd = [str(bridge), task.role]
    for file_path in task.files:
        cmd.extend(["--file", file_path])
    if task.prompt:
        cmd.extend(["--", task.prompt])
    return cmd


def terminate_process(proc: subprocess.Popen[Any]) -> None:
    if proc.poll() is not None:
        return
    try:
        os.killpg(proc.pid, signal.SIGTERM)
    except ProcessLookupError:
        return
    try:
        proc.wait(timeout=3)
    except subprocess.TimeoutExpired:
        try:
            os.killpg(proc.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        try:
            proc.wait(timeout=2)
        except subprocess.TimeoutExpired:
            pass


def run_task(
    bridge: Path,
    task: Task,
    task_timeout: float,
    cancel_event: threading.Event,
) -> dict[str, Any]:
    started = time.monotonic()
    cmd = build_command(bridge, task)
    result: dict[str, Any] = {
        "id": task.id,
        "role": task.role,
        "status": "error",
        "exit_code": None,
        "timed_out": False,
        "duration_sec": 0.0,
        "stdout": "",
        "stderr": "",
        "error": None,
    }

    if cancel_event.is_set():
        result["status"] = "cancelled"
        result["error"] = "cancelled before start"
        result["duration_sec"] = round(time.monotonic() - started, 3)
        return result

    try:
        proc = subprocess.Popen(
            cmd,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            start_new_session=True,
        )
    except OSError as exc:
        result["error"] = str(exc)
        result["duration_sec"] = round(time.monotonic() - started, 3)
        return result

    deadline = started + task_timeout
    stdout_data = ""
    stderr_data = ""
    timed_out = False
    cancelled = False

    try:
        while True:
            if cancel_event.is_set():
                cancelled = True
                terminate_process(proc)
                break
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                timed_out = True
                terminate_process(proc)
                break
            try:
                stdout_data, stderr_data = proc.communicate(timeout=min(0.2, remaining))
                break
            except subprocess.TimeoutExpired:
                continue
    finally:
        if proc.poll() is None:
            terminate_process(proc)
            if not stdout_data and not stderr_data:
                try:
                    out, err = proc.communicate(timeout=1)
                    stdout_data = out or ""
                    stderr_data = err or ""
                except subprocess.TimeoutExpired:
                    pass

    duration = round(time.monotonic() - started, 3)
    result["duration_sec"] = duration
    result["stdout"] = stdout_data
    result["stderr"] = stderr_data
    result["exit_code"] = proc.returncode
    result["timed_out"] = timed_out

    if cancelled:
        result["status"] = "cancelled"
        result["error"] = "cancelled"
    elif timed_out:
        result["status"] = "timeout"
        result["error"] = f"task exceeded {task_timeout}s"
    elif proc.returncode == 0:
        result["status"] = "ok"
    else:
        result["status"] = "error"
        result["error"] = f"exit code {proc.returncode}"

    return result


def run_swarm(
    tasks: list[Task],
    bridge: Path,
    max_workers: int,
    task_timeout: float,
    total_timeout: float,
) -> tuple[list[dict[str, Any]], str]:
    cancel_event = threading.Event()
    results: dict[str, dict[str, Any]] = {}
    lock = threading.Lock()

    overall_status = "ok"
    started = time.monotonic()

    def _handle_signal(signum: int, _frame: Any) -> None:
        cancel_event.set()
        nonlocal overall_status
        overall_status = "cancelled"

    previous_int = signal.signal(signal.SIGINT, _handle_signal)
    previous_term = signal.signal(signal.SIGTERM, _handle_signal)

    try:
        with ThreadPoolExecutor(max_workers=max_workers) as executor:
            futures = {
                executor.submit(run_task, bridge, task, task_timeout, cancel_event): task
                for task in tasks
            }
            pending = set(futures.keys())
            while pending:
                remaining = total_timeout - (time.monotonic() - started)
                if remaining <= 0:
                    cancel_event.set()
                    overall_status = "timeout"
                    for fut in pending:
                        fut.cancel()
                    # Wait for in-flight workers to notice cancel_event, then
                    # prefer real task results over synthetic placeholders.
                    done_late, still_pending = wait(
                        pending, timeout=min(task_timeout + 5, 30)
                    )
                    for fut in done_late:
                        task = futures[fut]
                        try:
                            result = fut.result()
                        except Exception as exc:  # noqa: BLE001
                            result = {
                                "id": task.id,
                                "role": task.role,
                                "status": "error",
                                "exit_code": None,
                                "timed_out": False,
                                "duration_sec": 0.0,
                                "stdout": "",
                                "stderr": "",
                                "error": str(exc),
                            }
                        with lock:
                            results[task.id] = result
                    for fut in still_pending:
                        task = futures[fut]
                        with lock:
                            if task.id not in results:
                                results[task.id] = {
                                    "id": task.id,
                                    "role": task.role,
                                    "status": "timeout",
                                    "exit_code": None,
                                    "timed_out": True,
                                    "duration_sec": round(
                                        time.monotonic() - started, 3
                                    ),
                                    "stdout": "",
                                    "stderr": "",
                                    "error": f"swarm exceeded {total_timeout}s",
                                }
                    break

                done, pending = wait(
                    pending, timeout=min(0.5, max(remaining, 0.01)), return_when=FIRST_COMPLETED
                )
                for fut in done:
                    task = futures[fut]
                    try:
                        result = fut.result()
                    except Exception as exc:  # noqa: BLE001 — surface as task error
                        result = {
                            "id": task.id,
                            "role": task.role,
                            "status": "error",
                            "exit_code": None,
                            "timed_out": False,
                            "duration_sec": 0.0,
                            "stdout": "",
                            "stderr": "",
                            "error": str(exc),
                        }
                    with lock:
                        results[task.id] = result
    finally:
        signal.signal(signal.SIGINT, previous_int)
        signal.signal(signal.SIGTERM, previous_term)

    ordered = [results.get(task.id) or missing_result(task) for task in tasks]
    if overall_status == "ok":
        if any(r["status"] != "ok" for r in ordered):
            overall_status = "partial"
    return ordered, overall_status


def missing_result(task: Task) -> dict[str, Any]:
    return {
        "id": task.id,
        "role": task.role,
        "status": "error",
        "exit_code": None,
        "timed_out": False,
        "duration_sec": 0.0,
        "stdout": "",
        "stderr": "",
        "error": "no result recorded",
    }


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
        help="JSON file: array of tasks or {\"tasks\": [...]}",
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
        bridge=bridge,
        max_workers=args.max_workers,
        task_timeout=args.task_timeout,
        total_timeout=args.total_timeout,
    )

    envelope = {
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
    if overall_status in {"partial", "timeout", "cancelled"}:
        return EXIT_PARTIAL
    return EXIT_PARTIAL


if __name__ == "__main__":
    sys.exit(main())
