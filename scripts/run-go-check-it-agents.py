#!/usr/bin/env python3
"""run-go-check-it-agents.py — tiered small-model quality pipeline.

Cross-IDE entry point:
  1. Parse go-check-it --format=agent-json
  2. Cluster hotspots into at most 6 non-local small-quality-worker tasks
  3. Fan out workers (OpenCode + GO_CHECK_IT_SMALL_MODEL)
  4. Ask small-go-check-it-orchestrator to synthesize an evidence packet
  5. Validate the packet; exclude raw transcripts from stdout

Primary IDE agent owns all edits. This script never mutates the repo.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
import tempfile
from pathlib import Path
from typing import Any

SCRIPT_DIR = Path(__file__).resolve().parent
sys.path.insert(0, str(SCRIPT_DIR))

from lib.swarm_runtime import SwarmTask, run_swarm  # noqa: E402

DEFAULT_MAX_WORKERS = 2
HARD_MAX_WORKERS = 6
MAX_CLOUD_TASKS = 6
DEFAULT_TASK_TIMEOUT = 180
DEFAULT_TOTAL_TIMEOUT = 600
MAX_CONTEXT_BYTES = 512 * 1024

EXIT_OK = 0
EXIT_PARTIAL = 1
EXIT_USAGE = 2

ESCALATE_KEYWORDS = (
    "race",
    "concurrent",
    "goroutine",
    "security",
    "auth",
    "password",
    "token",
    "public api",
    "breaking change",
)


class PipelineError(Exception):
    """Invalid input or configuration."""


def shutil_which(name: str) -> str | None:
    for directory in os.environ.get("PATH", "").split(os.pathsep):
        if not directory:
            continue
        candidate = Path(directory) / name
        if candidate.is_file() and os.access(candidate, os.X_OK):
            return str(candidate)
    return None


def resolve_bridge(env_name: str, sibling: str, path_name: str) -> Path:
    env = os.environ.get(env_name)
    if env:
        path = Path(env)
        if path.is_file() and os.access(path, os.X_OK):
            return path
        raise PipelineError(f"{env_name} is not executable: {env}")

    candidate = SCRIPT_DIR / sibling
    if candidate.is_file() and os.access(candidate, os.X_OK):
        return candidate

    which = shutil_which(path_name)
    if which:
        return Path(which)
    raise PipelineError(
        f"{path_name} not found; install with: sh scripts/install-path.sh"
    )


def load_agent_json(path: Path) -> dict[str, Any]:
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except OSError as exc:
        raise PipelineError(f"cannot read agent-json: {exc}") from exc
    except json.JSONDecodeError as exc:
        raise PipelineError(f"agent-json is not valid JSON: {exc}") from exc
    if not isinstance(data, dict):
        raise PipelineError("agent-json must be a JSON object")
    if "summary" not in data or "hotspots" not in data:
        raise PipelineError("agent-json missing summary/hotspots")
    if not isinstance(data.get("hotspots"), list):
        raise PipelineError("agent-json hotspots must be an array")
    return data


def cluster_hotspots(hotspots: list[dict[str, Any]], limit: int = MAX_CLOUD_TASKS) -> list[dict[str, Any]]:
    """Group hotspots by package then file; emit at most `limit` clusters."""
    groups: dict[str, dict[str, Any]] = {}
    order: list[str] = []
    for idx, spot in enumerate(hotspots):
        if not isinstance(spot, dict):
            continue
        package = str(spot.get("package") or "unknown")
        file_path = str(spot.get("file") or "unknown")
        key = f"{package}|{file_path}"
        if key not in groups:
            groups[key] = {
                "id": f"cluster-{len(order) + 1}",
                "package": package,
                "file": file_path,
                "hotspots": [],
            }
            order.append(key)
        groups[key]["hotspots"].append(spot)

    clusters = [groups[k] for k in order]
    selected = clusters[:limit]
    omitted = max(0, len(clusters) - len(selected))
    # Also count hotspots not covered when clusters exceed limit.
    selected_hotspot_count = sum(len(c["hotspots"]) for c in selected)
    omitted_hotspots = max(0, len(hotspots) - selected_hotspot_count)
    for cluster in selected:
        cluster["omitted_clusters"] = omitted
        cluster["omitted_hotspots"] = omitted_hotspots
    return selected


def extract_json_object(text: str) -> dict[str, Any] | None:
    """Best-effort extraction of a JSON object from OpenCode JSON event output."""
    text = text.strip()
    if not text:
        return None
    # Prefer the last JSON object in the stream.
    candidates: list[str] = []
    if text.startswith("{") and text.endswith("}"):
        candidates.append(text)
    # Scan for fenced or embedded objects.
    for match in re.finditer(r"\{[\s\S]*\}", text):
        candidates.append(match.group(0))
    for candidate in reversed(candidates):
        try:
            data = json.loads(candidate)
        except json.JSONDecodeError:
            continue
        if isinstance(data, dict):
            return data
    # OpenCode --format json often emits NDJSON events; look for text fields.
    for line in reversed(text.splitlines()):
        line = line.strip()
        if not line.startswith("{"):
            continue
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        if not isinstance(event, dict):
            continue
        for key in ("text", "content", "message", "part"):
            value = event.get(key)
            if isinstance(value, str):
                nested = extract_json_object(value)
                if nested is not None:
                    return nested
            if isinstance(value, dict) and "text" in value and isinstance(value["text"], str):
                nested = extract_json_object(value["text"])
                if nested is not None:
                    return nested
    return None


def normalize_worker_result(raw: dict[str, Any], cluster: dict[str, Any]) -> dict[str, Any]:
    parsed = extract_json_object(raw.get("stdout") or "")
    escalate_reasons: list[str] = []
    status = "error"
    diagnosis = ""
    proposed_diff = ""
    evidence: list[Any] = []
    focused_commands: list[str] = []
    confidence = 0
    unresolved: list[str] = []

    if parsed is not None:
        status = str(parsed.get("status") or "ok")
        diagnosis = str(parsed.get("diagnosis") or "")
        proposed_diff = str(parsed.get("proposed_diff") or "")
        evidence = parsed.get("evidence") if isinstance(parsed.get("evidence"), list) else []
        focused_commands = (
            parsed.get("focused_commands")
            if isinstance(parsed.get("focused_commands"), list)
            else []
        )
        try:
            confidence = int(parsed.get("confidence") or 0)
        except (TypeError, ValueError):
            confidence = 0
        escalate_reasons = [
            str(x)
            for x in (parsed.get("escalate_reasons") or [])
            if isinstance(x, (str, int, float))
        ]
        unresolved = [
            str(x)
            for x in (parsed.get("unresolved_risks") or [])
            if isinstance(x, (str, int, float))
        ]
    else:
        escalate_reasons.append("worker output was not valid JSON")
        status = "escalate"
        # Keep diagnosis compact; raw stdout/stderr stay in raw_logs_dir only.
        diagnosis = "worker returned non-JSON output; see raw_logs_dir"

    if raw.get("status") != "ok":
        status = "escalate"
        escalate_reasons.append(f"worker bridge status={raw.get('status')}")

    blob = f"{diagnosis} {proposed_diff}".lower()
    for keyword in ESCALATE_KEYWORDS:
        if keyword in blob and keyword not in " ".join(escalate_reasons).lower():
            escalate_reasons.append(f"keyword:{keyword}")
            status = "escalate"

    if confidence and confidence < 6 and status == "ok":
        status = "escalate"
        escalate_reasons.append("confidence below 6")

    return {
        "task_id": cluster["id"],
        "package": cluster["package"],
        "file": cluster["file"],
        "hotspot_count": len(cluster["hotspots"]),
        "hotspots": cluster["hotspots"],
        "bridge_status": raw.get("status"),
        "status": status,
        "diagnosis": diagnosis,
        "evidence": evidence,
        "proposed_diff": proposed_diff,
        "focused_commands": [str(c) for c in focused_commands if isinstance(c, str)],
        "confidence": confidence,
        "escalate_reasons": escalate_reasons,
        "unresolved_risks": unresolved,
        "duration_sec": raw.get("duration_sec"),
    }


REQUIRED_PACKET_KEYS = {
    "ok",
    "status",
    "summary",
    "recommendations",
    "disagreements",
    "unresolved_risks",
    "escalate_reasons",
    "omitted",
}


def validate_evidence_packet(packet: dict[str, Any]) -> list[str]:
    errors: list[str] = []
    missing = REQUIRED_PACKET_KEYS - set(packet)
    if missing:
        errors.append(f"missing keys: {sorted(missing)}")
    if "recommendations" in packet and not isinstance(packet["recommendations"], list):
        errors.append("recommendations must be an array")
    if "status" in packet and packet["status"] not in {"ready", "escalate", "partial"}:
        errors.append("status must be ready|escalate|partial")
    # Reject accidental transcript dumps.
    dumped = json.dumps(packet)
    if "WARNING: DATA RACE" in dumped and len(dumped) > 20000:
        errors.append("packet appears to contain raw race transcript")
    return errors


def fallback_packet(
    workers: list[dict[str, Any]],
    agent_json: dict[str, Any],
    raw_logs_dir: str,
    reason: str,
) -> dict[str, Any]:
    recommendations = []
    escalate_reasons = [reason]
    disagreements: list[str] = []
    for w in workers:
        action = "manual_review"
        if w.get("proposed_diff") and w.get("status") == "ok":
            action = "apply_diff"
        if w.get("status") == "escalate":
            escalate_reasons.extend(w.get("escalate_reasons") or [])
            action = "manual_review"
        recommendations.append(
            {
                "id": w["task_id"],
                "action": action,
                "diagnosis": w.get("diagnosis") or "",
                "evidence": w.get("evidence") or [],
                "proposed_diff": w.get("proposed_diff") or "",
                "focused_commands": w.get("focused_commands") or [],
                "confidence": w.get("confidence") or 0,
                "worker_ids": [w["task_id"]],
            }
        )
    omitted = agent_json.get("omitted") or {}
    summary = agent_json.get("summary") or {}
    status = "escalate" if escalate_reasons else "ready"
    if any(w.get("bridge_status") != "ok" for w in workers):
        status = "partial"
    return {
        "ok": status == "ready",
        "status": status,
        "summary": (
            f"Deterministic fallback packet ({reason}). "
            f"maxCrap={summary.get('maxCrap')} aboveThreshold="
            f"{summary.get('aboveThresholdCount')} findings="
            f"{summary.get('findingCount')}"
        ),
        "recommendations": recommendations,
        "disagreements": disagreements,
        "unresolved_risks": [],
        "escalate_reasons": sorted(set(str(x) for x in escalate_reasons if x)),
        "omitted": {
            "hotspots": omitted.get("functionsTotalOmitted", 0),
            "findings": 0,
            "clusters": workers[0].get("omitted_clusters", 0) if workers else 0,
        },
        "raw_logs_dir": raw_logs_dir,
    }


def write_json(path: Path, data: Any) -> None:
    path.write_text(json.dumps(data, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")


def build_bridge_command(bridge: Path, task: SwarmTask) -> list[str]:
    cmd = [str(bridge), task.role]
    for file_path in task.files:
        cmd.extend(["--file", file_path])
    if task.prompt:
        cmd.extend(["--", task.prompt])
    return cmd


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        prog="run-go-check-it-agents",
        description=(
            "Cluster go-check-it agent-json hotspots, run up to 6 small "
            "non-local workers, then synthesize an evidence packet."
        ),
    )
    parser.add_argument(
        "--agent-json",
        required=True,
        type=Path,
        help="Path to go-check-it --format=agent-json output",
    )
    parser.add_argument(
        "--max-workers",
        type=int,
        default=DEFAULT_MAX_WORKERS,
        help=f"parallel cloud workers (default {DEFAULT_MAX_WORKERS}, hard max {HARD_MAX_WORKERS})",
    )
    parser.add_argument(
        "--task-timeout",
        type=float,
        default=DEFAULT_TASK_TIMEOUT,
        help=f"per-worker timeout seconds (default {DEFAULT_TASK_TIMEOUT})",
    )
    parser.add_argument(
        "--total-timeout",
        type=float,
        default=DEFAULT_TOTAL_TIMEOUT,
        help=f"overall timeout seconds (default {DEFAULT_TOTAL_TIMEOUT})",
    )
    parser.add_argument(
        "--skip-orchestrator",
        action="store_true",
        help="Skip small orchestrator and emit deterministic fallback packet",
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
    if not os.environ.get("GO_CHECK_IT_SMALL_MODEL"):
        print(
            "error: GO_CHECK_IT_SMALL_MODEL is required (provider/model)",
            file=sys.stderr,
        )
        return EXIT_USAGE

    try:
        agent_json = load_agent_json(args.agent_json)
        small_bridge = resolve_bridge(
            "GO_CHECK_IT_SMALL_SUBAGENT_BIN",
            "run-small-subagent.sh",
            "run-small-subagent",
        )
    except PipelineError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return EXIT_USAGE

    hotspots = agent_json.get("hotspots") or []
    findings = agent_json.get("findings") or []
    clusters = cluster_hotspots(hotspots, MAX_CLOUD_TASKS)

    run_dir = Path(tempfile.mkdtemp(prefix="go-check-it-agents-"))
    write_json(run_dir / "agent.json", agent_json)

    if not clusters and not findings:
        packet = {
            "ok": True,
            "status": "ready",
            "summary": "No CRAP hotspots or practice findings in agent-json.",
            "recommendations": [],
            "disagreements": [],
            "unresolved_risks": [],
            "escalate_reasons": [],
            "omitted": {"hotspots": 0, "findings": 0, "clusters": 0},
            "raw_logs_dir": str(run_dir),
        }
        json.dump(packet, sys.stdout, indent=2, ensure_ascii=False)
        sys.stdout.write("\n")
        return EXIT_OK

    tasks: list[SwarmTask] = []
    for cluster in clusters:
        context_path = run_dir / f"{cluster['id']}.json"
        write_json(
            context_path,
            {
                "summary": agent_json.get("summary"),
                "cluster": cluster,
                "related_findings": [
                    f
                    for f in findings
                    if isinstance(f, dict)
                    and str(f.get("file") or "") == cluster["file"]
                ],
            },
        )
        size = context_path.stat().st_size
        if size > MAX_CONTEXT_BYTES:
            print(
                f"error: context for {cluster['id']} too large ({size} bytes)",
                file=sys.stderr,
            )
            return EXIT_USAGE
        names = ", ".join(
            str(h.get("functionName") or "?") for h in cluster["hotspots"][:5]
        )
        prompt = (
            f"task_id={cluster['id']}. Analyze hotspot cluster in package "
            f"{cluster['package']} file {cluster['file']} covering: {names}. "
            "Return the required JSON object only."
        )
        tasks.append(
            SwarmTask(
                id=cluster["id"],
                role="small-quality-worker",
                prompt=prompt,
                files=[str(context_path)],
            )
        )

    # If there are findings but no hotspot clusters, still ask one worker.
    if not tasks and findings:
        context_path = run_dir / "findings-only.json"
        write_json(context_path, {"summary": agent_json.get("summary"), "findings": findings})
        tasks.append(
            SwarmTask(
                id="findings-1",
                role="small-quality-worker",
                prompt="task_id=findings-1. Diagnose practice findings only. Return JSON.",
                files=[str(context_path)],
            )
        )
        clusters = [
            {
                "id": "findings-1",
                "package": "",
                "file": "",
                "hotspots": [],
                "omitted_clusters": 0,
                "omitted_hotspots": 0,
            }
        ]

    raw_results, overall = run_swarm(
        tasks=tasks,
        max_workers=min(args.max_workers, len(tasks)),
        task_timeout=args.task_timeout,
        total_timeout=args.total_timeout,
        build_command=lambda task: build_bridge_command(small_bridge, task),
    )
    write_json(run_dir / "raw-workers.json", raw_results)

    cluster_by_id = {c["id"]: c for c in clusters}
    normalized = []
    for raw in raw_results:
        cluster = cluster_by_id.get(raw["id"]) or {
            "id": raw["id"],
            "package": "",
            "file": "",
            "hotspots": [],
            "omitted_clusters": 0,
            "omitted_hotspots": 0,
        }
        item = normalize_worker_result(raw, cluster)
        item["omitted_clusters"] = cluster.get("omitted_clusters", 0)
        item["omitted_hotspots"] = cluster.get("omitted_hotspots", 0)
        # Persist per-worker raw transcript privately.
        (run_dir / f"{raw['id']}.stdout").write_text(raw.get("stdout") or "", encoding="utf-8")
        (run_dir / f"{raw['id']}.stderr").write_text(raw.get("stderr") or "", encoding="utf-8")
        normalized.append(item)
    write_json(run_dir / "normalized-workers.json", normalized)

    packet: dict[str, Any]
    if args.skip_orchestrator:
        packet = fallback_packet(
            normalized, agent_json, str(run_dir), "orchestrator skipped"
        )
    else:
        orch_input = run_dir / "orchestrator-input.json"
        write_json(
            orch_input,
            {
                "agent_json": {
                    "summary": agent_json.get("summary"),
                    "omitted": agent_json.get("omitted"),
                    "finding_count": len(findings),
                },
                "workers": normalized,
                "raw_logs_dir": str(run_dir),
            },
        )
        orch_task = SwarmTask(
            id="orchestrator",
            role="small-go-check-it-orchestrator",
            prompt=(
                "Synthesize the attached worker results into the evidence packet "
                "JSON schema. Do not include raw transcripts."
            ),
            files=[str(orch_input)],
        )
        orch_results, orch_status = run_swarm(
            tasks=[orch_task],
            max_workers=1,
            task_timeout=args.task_timeout,
            total_timeout=min(args.total_timeout, args.task_timeout + 30),
            build_command=lambda task: build_bridge_command(small_bridge, task),
        )
        write_json(run_dir / "raw-orchestrator.json", orch_results)
        orch_raw = orch_results[0] if orch_results else {}
        (run_dir / "orchestrator.stdout").write_text(
            orch_raw.get("stdout") or "", encoding="utf-8"
        )
        (run_dir / "orchestrator.stderr").write_text(
            orch_raw.get("stderr") or "", encoding="utf-8"
        )
        parsed = extract_json_object(orch_raw.get("stdout") or "")
        if parsed is None or validate_evidence_packet(parsed):
            reason = "orchestrator packet invalid"
            if orch_status != "ok":
                reason = f"orchestrator status={orch_status}"
            elif parsed is None:
                reason = "orchestrator returned non-JSON"
            else:
                reason = "orchestrator schema errors: " + "; ".join(
                    validate_evidence_packet(parsed)
                )
            packet = fallback_packet(normalized, agent_json, str(run_dir), reason)
        else:
            packet = parsed
            packet["raw_logs_dir"] = str(run_dir)
            # Never trust workers' transcripts if the model echoed them.
            if "tasks" in packet:
                del packet["tasks"]

    errors = validate_evidence_packet(packet)
    if errors:
        packet = fallback_packet(
            normalized, agent_json, str(run_dir), "packet validation failed: " + "; ".join(errors)
        )

    write_json(run_dir / "evidence-packet.json", packet)
    json.dump(packet, sys.stdout, indent=2, ensure_ascii=False)
    sys.stdout.write("\n")

    if packet.get("status") == "ready" and packet.get("ok") is True and overall == "ok":
        return EXIT_OK
    return EXIT_PARTIAL


if __name__ == "__main__":
    sys.exit(main())
