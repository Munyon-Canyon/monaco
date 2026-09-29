#!/usr/bin/env python3
"""Fail a checkpoint PR into main that changes the backend without a CHANGELOG entry.

usage: scripts/check-changelog.py <base> <head>

Fails when `git diff <base>...<head>` touches apps/backend/** but not
apps/backend/CHANGELOG.md, or when the changelog at <head> has nothing to release: an
empty `## [Unreleased]` section and no `## [checkpoint N] - <date>` section that this
diff added with entries (the checkpoint PR renames [Unreleased] and opens a new, empty one).
Rules: apps/backend/CHANGELOG.md preamble and docs/architecture/ci.md#feature-branches.
"""

from __future__ import annotations

import re
import subprocess
import sys

CHANGELOG = "apps/backend/CHANGELOG.md"
BACKEND = "apps/backend/"
UNRELEASED = "[Unreleased]"
HEADINGS = "### Added, ### Changed, ### Fixed, ### Removed or ### Security"
PLACEHOLDER = "Nothing yet."
SECTION = re.compile(r"^## (\[[^\]]+\])")


def sections(text: str) -> dict[str, list[str]]:
    found: dict[str, list[str]] = {}
    current = None
    for line in text.splitlines():
        match = SECTION.match(line)
        if match:
            current = match.group(1)
            found.setdefault(current, [])
        elif current and line.startswith("- ") and line[2:].strip() != PLACEHOLDER:
            found[current].append(line)
    return found


def problems(changed: list[str], head_text: str | None, base_text: str | None) -> list[str]:
    backend = [p for p in changed if p.startswith(BACKEND) and p != CHANGELOG]
    if not backend:
        return []
    if CHANGELOG not in changed or head_text is None:
        return [
            f"{len(backend)} file(s) under {BACKEND} changed but {CHANGELOG} did not. "
            f"Add one line per change a user or operator would notice under ## {UNRELEASED}, "
            f"in {HEADINGS} (Keep a Changelog)."
        ]
    head = sections(head_text)
    if head.get(UNRELEASED):
        return []
    base = sections(base_text or "")
    released = [name for name in head if name != UNRELEASED]
    if released and released[0] not in base and head[released[0]]:
        return []
    return [
        f"## {UNRELEASED} in {CHANGELOG} has no entries. Add one line per change a user or "
        f"operator would notice, in {HEADINGS} (Keep a Changelog), or rename it to "
        f"## [checkpoint N] - <date> and open a new empty ## {UNRELEASED} above it."
    ]


def git(*args: str) -> str:
    return subprocess.run(["git", *args], check=True, capture_output=True, text=True).stdout


def show(rev: str) -> str | None:
    result = subprocess.run(["git", "show", f"{rev}:{CHANGELOG}"], capture_output=True, text=True)
    return result.stdout if result.returncode == 0 else None


def main(argv: list[str]) -> int:
    if len(argv) != 3:
        print(__doc__.strip().splitlines()[2], file=sys.stderr)
        return 2
    base, head = argv[1], argv[2]
    changed = git("diff", "--name-only", f"{base}...{head}").split()
    fork = git("merge-base", base, head).strip()
    found = problems(changed, show(head), show(fork))
    for problem in found:
        print(f"::error file={CHANGELOG}::{problem}")
    if not found:
        print(f"{CHANGELOG} covers this checkpoint.")
    return 1 if found else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
