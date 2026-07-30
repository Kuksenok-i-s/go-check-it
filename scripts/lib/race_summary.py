"""Deterministic Go race-detector log summarizer."""

from __future__ import annotations

import re
from dataclasses import dataclass, field


@dataclass
class RaceAccess:
    kind: str  # Read | Write
    address: str
    goroutine: str
    function: str = ""
    file: str = ""
    line: int = 0


@dataclass
class RaceSummary:
    test_name: str = ""
    race_count: int = 0
    accesses: list[RaceAccess] = field(default_factory=list)
    creation_stacks: list[str] = field(default_factory=list)
    notes: list[str] = field(default_factory=list)

    def to_dict(self) -> dict:
        return {
            "test_name": self.test_name,
            "race_count": self.race_count,
            "accesses": [
                {
                    "kind": a.kind,
                    "address": a.address,
                    "goroutine": a.goroutine,
                    "function": a.function,
                    "file": a.file,
                    "line": a.line,
                }
                for a in self.accesses
            ],
            "creation_stacks": self.creation_stacks,
            "notes": self.notes,
        }


_WARNING = re.compile(r"^WARNING: DATA RACE\s*$", re.M)
_ACCESS = re.compile(
    r"^(?:Previous )?(Read|Write) at (0x[0-9a-fA-F]+) by goroutine (\d+):\s*$",
    re.M | re.I,
)
_FRAME = re.compile(
    r"^\s+([^\n]+?)\(\)\n\s+([^\s:]+):(\d+)",
    re.M,
)
_CREATED = re.compile(
    r"^Goroutine (\d+) \(.*?\) created at:\s*\n\s+([^\n]+)\n\s+([^\s:]+):(\d+)",
    re.M,
)
_FAIL = re.compile(r"^--- FAIL: (\S+)", re.M)


def summarize_race_log(text: str) -> RaceSummary:
    """Extract a compact summary from Go race detector output.

    Preserves the first competing access pair and creation stacks when present.
    Full logs should still be kept outside the chat for audit.
    """
    summary = RaceSummary()
    summary.race_count = len(_WARNING.findall(text))
    fails = _FAIL.findall(text)
    if fails:
        summary.test_name = fails[0]

    for match in _ACCESS.finditer(text):
        kind, address, goroutine = match.groups()
        access = RaceAccess(kind=kind.title(), address=address, goroutine=goroutine)
        rest = text[match.end() : match.end() + 400]
        frame = _FRAME.search(rest)
        if frame:
            access.function = frame.group(1).strip()
            access.file = frame.group(2).strip()
            access.line = int(frame.group(3))
        summary.accesses.append(access)
        if len(summary.accesses) >= 4:
            break

    for match in _CREATED.finditer(text):
        goroutine, fn, file, line = match.groups()
        summary.creation_stacks.append(
            f"goroutine {goroutine} created at {fn} ({file}:{line})"
        )
        if len(summary.creation_stacks) >= 4:
            break

    if summary.race_count == 0 and "DATA RACE" not in text:
        summary.notes.append("no DATA RACE markers found")
    elif not summary.accesses:
        summary.notes.append("race markers found but stacks could not be parsed")
    return summary
