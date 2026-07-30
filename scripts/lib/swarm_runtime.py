"""Shared bounded fan-out runtime for go-check-it agent bridges."""

from __future__ import annotations

import os
import signal
import subprocess
import threading
import time
from concurrent.futures import FIRST_COMPLETED, ThreadPoolExecutor, wait
from dataclasses import dataclass, field
from typing import Any, Callable


@dataclass
class SwarmTask:
    id: str
    role: str
    prompt: str = ""
    files: list[str] = field(default_factory=list)
    command: list[str] | None = None


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
    task: SwarmTask,
    task_timeout: float,
    cancel_event: threading.Event,
    build_command: Callable[[SwarmTask], list[str]] | None = None,
) -> dict[str, Any]:
    started = time.monotonic()
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

    if task.command is not None:
        cmd = list(task.command)
    elif build_command is not None:
        cmd = build_command(task)
    else:
        result["error"] = "no command builder"
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


def missing_result(task: SwarmTask) -> dict[str, Any]:
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


def run_swarm(
    tasks: list[SwarmTask],
    max_workers: int,
    task_timeout: float,
    total_timeout: float,
    build_command: Callable[[SwarmTask], list[str]] | None = None,
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
                executor.submit(
                    run_task, task, task_timeout, cancel_event, build_command
                ): task
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
                    pending,
                    timeout=min(0.5, max(remaining, 0.01)),
                    return_when=FIRST_COMPLETED,
                )
                for fut in done:
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
    finally:
        signal.signal(signal.SIGINT, previous_int)
        signal.signal(signal.SIGTERM, previous_term)

    ordered = [results.get(task.id) or missing_result(task) for task in tasks]
    if overall_status == "ok":
        if any(r["status"] != "ok" for r in ordered):
            overall_status = "partial"
    return ordered, overall_status
