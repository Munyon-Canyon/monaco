#!/usr/bin/env python3
"""Fail a pull request with 1000 or more changed lines of hand-written code, tests or docs,
or with a committed binary file.

Counts added plus deleted lines from `git diff --numstat -M <base>...<head>`, so a pure
rename counts as zero. Skips generated and machine-written files (see IGNORED). A PR
labelled `large-pr` passes; only a human reviewer adds that label. Each PR of a Graphite
stack is measured against its own parent, the PR below it. A binary file fails the PR whatever its size or label, unless it
sits under a `testdata/` directory or has a media or document extension (MEDIA_SUFFIXES).
The rule stops committed build output, which has no such extension.
A PR labelled `fast-track` fails at 100 or more counted lines or when it touches a FAST_TRACK_HEAVY
path, and writes `fast_track_too_big=true` to GITHUB_OUTPUT so the workflow removes the label. The
Graphite merge queue lets a fast-track PR jump the line, and a jump is safe when the PR cannot break
what it passes: a small change outside the backend and CI, whose stage 2 runs nothing heavy.

Reads BASE_SHA, HEAD_SHA and PR_LABELS (JSON list of label names) from the environment.
Rules: docs/architecture/backend-platform.md#pull-requests-small-and-stacked
"""

from __future__ import annotations

import fnmatch
import json
import os
import subprocess
import sys

LIMIT = 1000
OVERRIDE_LABEL = "large-pr"
FAST_TRACK_LABEL = "fast-track"
FAST_TRACK_LIMIT = 100
FAST_TRACK_HEAVY = ("apps/backend/", ".github/", "docker-compose.yml")
IGNORED = [
    "*.gen.go",
    "*_gen.go",
    "apps/backend/api/openapi.yaml",
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
MEDIA_SUFFIXES = (
    ".png", ".jpg", ".jpeg", ".gif", ".webp", ".ico", ".svg", ".pdf",
    ".ttf", ".otf", ".woff", ".woff2", ".mp4", ".mov",
)


def renamed_path(path: str) -> str:
    if "=>" not in path:
        return path
    if "{" in path:
        head, rest = path.split("{", 1)
        inner, tail = rest.split("}", 1)
        return head + inner.split("=>")[1].strip() + tail
    return path.split("=>")[1].strip()


def both_paths(path: str) -> list[str]:
    if "=>" not in path:
        return [path]
    if "{" in path:
        head, rest = path.split("{", 1)
        inner, tail = rest.split("}", 1)
        return [head + side.strip() + tail for side in inner.split("=>")]
    return [side.strip() for side in path.split("=>")]


def touched(numstat: str) -> list[str]:
    """Every path the diff touches, ignored and binary files and both sides of a rename included."""
    paths = []
    for line in numstat.splitlines():
        parts = line.split("\t", 2)
        if len(parts) == 3:
            paths.extend(both_paths(parts[2]))
    return paths


def ignored(path: str) -> bool:
    return any(fnmatch.fnmatch(path, pattern) for pattern in IGNORED)


def binary_allowed(path: str) -> bool:
    if "/testdata/" in "/" + path:
        return True
    return path.lower().endswith(MEDIA_SUFFIXES)


def binaries(numstat: str) -> list[str]:
    found = []
    for line in numstat.splitlines():
        parts = line.split("\t", 2)
        if len(parts) == 3 and parts[0] == "-" and parts[1] == "-":
            path = renamed_path(parts[2])
            if not binary_allowed(path):
                found.append(path)
    return found


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


def numstat(base: str, head: str) -> str:
    for sha in (base, head):
        if subprocess.run(["git", "cat-file", "-e", f"{sha}^{{commit}}"], capture_output=True).returncode != 0:
            subprocess.run(["git", "fetch", "--quiet", "--no-tags", "origin", sha], check=True)
    return subprocess.run(
        ["git", "diff", "--numstat", "-M", f"{base}...{head}"],
        capture_output=True,
        text=True,
        check=True,
    ).stdout


def main() -> int:
    labels = json.loads(os.environ.get("PR_LABELS") or "[]")
    diff = numstat(os.environ["BASE_SHA"], os.environ["HEAD_SHA"])
    rejected = binaries(diff)
    if rejected:
        print("PR commits binary files. Remove them; build output belongs in bin/ or /dev/null:")
        for path in rejected:
            print(f"  {path}")
        return 1
    total, counted = count(diff)
    print(f"{total} changed lines counted (limit {LIMIT}). Largest files:")
    for lines, path in counted[:10]:
        print(f"  {lines:6d}  {path}")
    heavy = [path for path in touched(diff) if path.startswith(FAST_TRACK_HEAVY)]
    if FAST_TRACK_LABEL in labels and (total >= FAST_TRACK_LIMIT or heavy):
        why = f"it touches {', '.join(heavy[:3])}" if heavy else f"it has {total} lines"
        print(f"The `{FAST_TRACK_LABEL}` label is for PRs under {FAST_TRACK_LIMIT} lines outside "
              f"{', '.join(FAST_TRACK_HEAVY)}; {why}. The label comes off; the PR waits its turn in the merge queue.")
        if out := os.environ.get("GITHUB_OUTPUT"):
            with open(out, "a") as f:
                f.write("fast_track_too_big=true\n")
        return 1
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
