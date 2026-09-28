#!/usr/bin/env python3
"""Fail a pull request with 1000 or more changed lines of hand-written code, tests or docs.

Counts added plus deleted lines from `git diff --numstat -M <base>...<head>`, so a pure
rename counts as zero. Skips generated and machine-written files (see IGNORED). A PR
labelled `large-pr` passes; only a human reviewer adds that label.

Reads BASE_SHA, HEAD_SHA and PR_LABELS (JSON list of label names) from the environment.
Rules: docs/architecture/backend-platform.md#pull-requests-small-and-stacked
"""

import fnmatch
import json
import os
import subprocess
import sys

LIMIT = 1000
OVERRIDE_LABEL = "large-pr"
IGNORED = [
    "*.gen.go",
    "*_gen.go",
    "apps/backend/.golangci.yml",
    "*.pb.go",
    "go.sum",
    "*/go.sum",
    "package-lock.json",
    "*/package-lock.json",
    "pnpm-lock.yaml",
    "*/pnpm-lock.yaml",
    "*.png",
    "*.jpg",
    "*.jpeg",
    "*.gif",
    "*.pdf",
    "*/test/evidence/*",
    "test/evidence/*",
    "docs/reference/*",
    "*/testdata/*",
]


def renamed_path(path: str) -> str:
    if "=>" not in path:
        return path
    if "{" in path:
        head, rest = path.split("{", 1)
        inner, tail = rest.split("}", 1)
        return head + inner.split("=>")[1].strip() + tail
    return path.split("=>")[1].strip()


def ignored(path: str) -> bool:
    return any(fnmatch.fnmatch(path, pattern) for pattern in IGNORED)


def count(numstat: str) -> tuple[int, list[tuple[int, str]]]:
    total = 0
    counted = []
    for line in numstat.splitlines():
        parts = line.split("\t", 2)
        if len(parts) != 3 or parts[0] == "-":
            continue
        path = renamed_path(parts[2])
        if ignored(path):
            continue
        lines = int(parts[0]) + int(parts[1])
        total += lines
        counted.append((lines, path))
    return total, sorted(counted, reverse=True)


def main() -> int:
    labels = json.loads(os.environ.get("PR_LABELS") or "[]")
    numstat = subprocess.run(
        ["git", "diff", "--numstat", "-M", f"{os.environ['BASE_SHA']}...{os.environ['HEAD_SHA']}"],
        capture_output=True,
        text=True,
        check=True,
    ).stdout
    total, counted = count(numstat)
    print(f"{total} changed lines counted (limit {LIMIT}). Largest files:")
    for lines, path in counted[:10]:
        print(f"  {lines:6d}  {path}")
    if total < LIMIT:
        return 0
    if OVERRIDE_LABEL in labels:
        print(f"Over the limit, allowed by the `{OVERRIDE_LABEL}` label.")
        return 0
    print(f"PR is over the {LIMIT}-line limit. Split it into a Graphite stack "
          "(the distribute-stack-changes skill or `gt split --by-hunk`), or ask a human "
          f"reviewer for the `{OVERRIDE_LABEL}` label if the change is mechanical.")
    return 1


if __name__ == "__main__":
    sys.exit(main())
