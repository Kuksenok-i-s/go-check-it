#!/usr/bin/env bash
# Unit checks for scripts/lib/race_summary.py

set -euo pipefail

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(dirname -- "$script_dir")

python3 - <<PY
import sys
sys.path.insert(0, "$script_dir")
from lib.race_summary import summarize_race_log

sample = """
==================
WARNING: DATA RACE
Write at 0x00c0001a2010 by goroutine 8:
  example.com/app/internal/cache.(*Cache).Set()
      /tmp/app/internal/cache/cache.go:33 +0x104
Previous read at 0x00c0001a2010 by goroutine 9:
  example.com/app/internal/cache.(*Cache).Get()
      /tmp/app/internal/cache/cache.go:21 +0x88
Goroutine 8 (running) created at:
  example.com/app/internal/cache.TestCacheConcurrent()
      /tmp/app/internal/cache/cache_test.go:40 +0x200
==================
--- FAIL: TestCacheConcurrent (0.00s)
"""
summary = summarize_race_log(sample)
assert summary.race_count == 1, summary
assert summary.test_name == "TestCacheConcurrent", summary
assert len(summary.accesses) >= 2, summary
assert summary.accesses[0].kind == "Write"
assert summary.accesses[0].file.endswith("cache.go")
assert summary.accesses[0].line == 33
assert summary.accesses[1].kind == "Read"
assert summary.creation_stacks, summary
print("race-summary validation passed")
PY
