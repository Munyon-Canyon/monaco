#!/usr/bin/env python3
"""Fail when a file on the frozen legacy list grows between BASE_SHA and HEAD_SHA.

packages/mobile-core/legacy-baseline.tsv lists where legacy mobile API code may live, and
LegacyFreezeTests keeps the patterns inside it. This check stores no counts: for each listed
path at HEAD_SHA it counts PATTERNS (and newlines, for LINE_COUNTED files) at base and head,
and reports `legacy code grew: <metric> <path> <base> -> <head>` for each count that rose.
A path absent at base counts from zero. Exits 1 on any finding unless PR_LABELS (a JSON list of
label names) holds gate-change-approved, which only a human adds.
Rules: docs/architecture/ci.md#what-runs-where
"""

from __future__ import annotations

import json
import os
import subprocess
import sys

OVERRIDE_LABEL = "gate-change-approved"
LIST = "packages/mobile-core/legacy-baseline.tsv"
API_TREE = "apps/mobile/Monaco/API/"
LINE_COUNTED = {
    "packages/mobile-core/Sources/MonacoCore/MonacoAPIClient.swift",
    "apps/mobile/Monaco/Features/Shell/AppSessionStore.swift",
}
PATTERNS = {
    "urlrequest": ["URLRequest("],
    "timer": ["Timer.publish"],
    "poll": ["pollWhileVisible(", "PollLoop.run("],
    "groups_path": ['"/v1/groups'],
}


def show(sha: str, path: str) -> str | None:
    result = subprocess.run(["git", "show", f"{sha}:{path}"], capture_output=True, text=True)
    return result.stdout if result.returncode == 0 else None


def counts(text: str, path: str) -> dict[str, int]:
    found = {metric: sum(text.count(p) for p in patterns) for metric, patterns in PATTERNS.items()}
    if path.startswith(API_TREE) or path in LINE_COUNTED:
        found["lines"] = text.count("\n")
    return found


def growth(base: str, head: str) -> list[str]:
    listed = (show(head, LIST) or "").splitlines()
    findings = []
    for path in filter(None, listed):
        after = show(head, path)
        if after is None:
            continue
        before = counts(show(base, path) or "", path)
        for metric, count in counts(after, path).items():
            if count > before[metric]:
                findings.append(f"legacy code grew: {metric} {path} {before[metric]} -> {count}")
    return findings


def main() -> int:
    findings = growth(os.environ["BASE_SHA"], os.environ["HEAD_SHA"])
    for finding in findings:
        print(finding)
    if not findings:
        return 0
    if OVERRIDE_LABEL in json.loads(os.environ.get("PR_LABELS") or "[]"):
        print(f"legacy growth approved by {OVERRIDE_LABEL}")
        return 0
    return 1


if __name__ == "__main__":
    sys.exit(main())
