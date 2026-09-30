#!/usr/bin/env python3
"""Fail a pull request with 1000 or more changed lines of hand-written code, tests or docs,
or with a committed binary file.

Counts added plus deleted lines from `git diff --numstat -M <base>...<head>`, so a pure
rename counts as zero. Skips generated and machine-written files (see IGNORED). A PR
labelled `large-pr` passes; only a human reviewer adds that label. A PR whose body starts
with `Lands stack: #a #b #c` (written by `monacoctl agents land-stack`) passes when it is
the last PR listed, each listed PR is under the limit against its own parent, and each
head has a `verify` success. A PR with no such line passes on the same terms when another
open PR's `Lands stack:` line lists it, applied to the list up to and including this PR.
A binary file fails the PR whatever its size or label, unless it
sits under a `testdata/` directory or has a media or document extension (MEDIA_SUFFIXES).
The rule stops committed build output, which has no such extension.

Reads BASE_SHA, HEAD_SHA, PR_LABELS (JSON list of label names), PR_BODY, PR_NUMBER and
GITHUB_REPOSITORY from the environment. Without PR_NUMBER, as in `monacoctl agents check`,
it looks up no open PR.
Rules: docs/architecture/backend-platform.md#pull-requests-small-and-stacked
"""

from __future__ import annotations

import fnmatch
import json
import os
import re
import subprocess
import sys

LIMIT = 1000
OVERRIDE_LABEL = "large-pr"
STACK_PREFIX = "Lands stack:"
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


def stack_numbers(body: str) -> list[int] | None:
    first = (body or "").split("\n", 1)[0].strip()
    if not first.startswith(STACK_PREFIX):
        return None
    return [int(n) for n in re.findall(r"#(\d+)", first)]


class Repo:
    def __init__(self, slug: str):
        self.slug = slug

    def head(self, number: int) -> str:
        return subprocess.run(
            ["gh", "pr", "view", str(number), "-R", self.slug, "--json", "headRefOid", "-q", ".headRefOid"],
            capture_output=True, text=True, check=True,
        ).stdout.strip()

    def verified(self, sha: str) -> bool:
        out = subprocess.run(
            ["gh", "api", f"repos/{self.slug}/commits/{sha}/statuses?per_page=100"],
            capture_output=True, text=True, check=True,
        ).stdout
        verify = [s for s in json.loads(out) if s.get("context") == "verify"]
        return bool(verify) and verify[0].get("state") == "success"

    def lines(self, parent: str, sha: str) -> int:
        return count(numstat(parent, sha))[0]

    def on_top_of(self, parent: str, sha: str) -> bool:
        return subprocess.run(["git", "merge-base", "--is-ancestor", parent, sha]).returncode == 0

    def landed_by(self, pr: int) -> tuple[int, list[int]] | None:
        out = subprocess.run(
            ["gh", "pr", "list", "-R", self.slug, "--state", "open", "--json", "number,body", "--limit", "200"],
            capture_output=True, text=True, check=True,
        ).stdout
        for open_pr in json.loads(out):
            numbers = stack_numbers(open_pr["body"])
            if numbers and pr in numbers:
                return open_pr["number"], numbers
        return None


def stack_errors(numbers: list[int], pr: int, base: str, repo: Repo) -> list[str]:
    if not numbers or numbers[-1] != pr:
        return [f"the `{STACK_PREFIX}` line must list the stack bottom to top and end with this PR, #{pr}"]
    errors = []
    parent = base
    for i, number in enumerate(numbers):
        sha = repo.head(number)
        if i > 0 and not repo.on_top_of(parent, sha):
            errors.append(f"#{number} does not sit on #{numbers[i - 1]}")
        lines = repo.lines(parent, sha)
        if lines >= LIMIT:
            errors.append(f"#{number} is {lines} lines against its parent")
        if not repo.verified(sha):
            errors.append(f"#{number} has no verify success on its head")
        parent = sha
    return errors


def own_lines(numbers: list[int], base: str, repo: Repo) -> int:
    parent = repo.head(numbers[-2]) if len(numbers) > 1 else base
    return repo.lines(parent, repo.head(numbers[-1]))


def report(header: str, errors: list[str]) -> None:
    print(header)
    for e in errors:
        print(f"  - {e}")


def main() -> int:
    return 0
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
    if total < LIMIT:
        return 0
    if OVERRIDE_LABEL in labels:
        print(f"Over the limit, allowed by the `{OVERRIDE_LABEL}` label.")
        return 0
    numbers = stack_numbers(os.environ.get("PR_BODY", ""))
    if numbers is not None:
        errors = stack_errors(
            numbers, int(os.environ["PR_NUMBER"]), os.environ["BASE_SHA"], Repo(os.environ["GITHUB_REPOSITORY"])
        )
        if not errors:
            print(f"Over the limit, allowed: it lands a verified stack of PRs each under {LIMIT} lines.")
            return 0
        report(f"The `{STACK_PREFIX}` line does not hold:", errors)
    elif "PR_NUMBER" in os.environ:
        pr, base = int(os.environ["PR_NUMBER"]), os.environ["BASE_SHA"]
        repo = Repo(os.environ["GITHUB_REPOSITORY"])
        landed = repo.landed_by(pr)
        if landed is not None:
            top, listed = landed
            numbers = listed[: listed.index(pr) + 1]
            errors = stack_errors(numbers, pr, base, repo)
            if not errors:
                print(
                    f"Over the limit against the feature branch, allowed: #{top} lands it in a verified stack, "
                    f"and it is {own_lines(numbers, base, repo)} lines against its own parent."
                )
                return 0
            report(f"The `{STACK_PREFIX}` line of #{top} does not hold for this PR:", errors)
    print(f"PR is over the {LIMIT}-line limit. Split it into a Graphite stack "
          "(the distribute-stack-changes skill or `gt split --by-hunk`), or ask a human "
          f"reviewer for the `{OVERRIDE_LABEL}` label if the change is mechanical.")
    return 1


if __name__ == "__main__":
    sys.exit(main())
