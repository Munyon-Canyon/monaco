#!/usr/bin/env python3
"""Fail a pull request that weakens a test gate, unless a human adds `gate-change-approved`.

Diffs BASE_SHA...HEAD_SHA and reports each finding as `path:line: <rule>: <what>`:

- gate-file: an added line in a gate file (coverage.exclude, mutants.allow, a perf baseline,
  a golden file, or the `exclusions:` block of apps/backend/.golangci.yml). A newly created
  baseline or golden file is not a finding: it adds a gate, it does not loosen one.
- test-skip: an added line in a *_test.go that calls t.Skip, t.Skipf, t.SkipNow or b.Skip*.
- test-removed: a Test, Fuzz or Benchmark function present at base and absent everywhere at
  head, unless its base file still has at least as many of them (a rename).

Reads BASE_SHA, HEAD_SHA and PR_LABELS (JSON list of label names) from the environment.
`--worktree [path...]` instead diffs the working tree (untracked files included) against HEAD,
limits findings to the given paths, skips the label check, and exits 1 on any finding. The
Claude Code hook scripts/agent-guard-gates.sh calls it after each edit.
Rules: docs/architecture/ci.md#what-runs-where
"""

from __future__ import annotations

import json
import os
import re
import subprocess
import sys
from collections import Counter
from dataclasses import dataclass

OVERRIDE_LABEL = "gate-change-approved"
GOLANGCI = "apps/backend/.golangci.yml"
GATE_FILES = {"apps/backend/coverage.exclude", "apps/backend/mutants.allow"}
NEW_FILE_GATES = re.compile(r"(^|/)testdata/(perf/baseline\.json$|golden/)")
SKIP_CALL = re.compile(r"\b[tb]\.Skip(f|Now)?\(")
TEST_FUNC = r"^func (Test|Fuzz|Benchmark)[A-Za-z0-9_]+\("
HUNK = re.compile(r"^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@")


@dataclass(frozen=True)
class Added:
    path: str
    line: int
    text: str
    new_file: bool


@dataclass(frozen=True)
class Finding:
    path: str
    line: int
    rule: str
    what: str

    def __str__(self) -> str:
        return f"{self.path}:{self.line}: {self.rule}: {self.what}"


def git(*args: str, ok=(0,)) -> str:
    result = subprocess.run(["git", *args], capture_output=True, text=True)
    if result.returncode not in ok:
        raise SystemExit(f"git {' '.join(args)} failed: {result.stderr.strip()}")
    return result.stdout


def added_lines(diff: str) -> list[Added]:
    added: list[Added] = []
    path, new_file, line = "", False, 0
    for raw in diff.splitlines():
        if raw.startswith("diff --git "):
            path, new_file = "", False
        elif raw.startswith("new file mode"):
            new_file = True
        elif raw.startswith("+++ "):
            path = raw[6:] if raw.startswith("+++ b/") else ""
        elif m := HUNK.match(raw):
            line = int(m.group(1))
        elif raw.startswith("+") and path:
            added.append(Added(path, line, raw[1:], new_file))
            line += 1
    return added


def exclusion_lines(yaml: str) -> set[int]:
    lines: set[int] = set()
    indent = None
    for number, text in enumerate(yaml.splitlines(), 1):
        stripped = text.lstrip()
        depth = len(text) - len(stripped)
        if indent is not None and stripped and depth <= indent:
            indent = None
        if indent is not None:
            lines.add(number)
        elif stripped.rstrip() == "exclusions:":
            indent = depth
    return lines


def gate_findings(added: list[Added], golangci_exclusions: set[int]) -> list[Finding]:
    findings = []
    for a in added:
        gated = (
            a.path in GATE_FILES
            or (NEW_FILE_GATES.search(a.path) and not a.new_file)
            or (a.path == GOLANGCI and a.line in golangci_exclusions)
        )
        if gated:
            findings.append(Finding(a.path, a.line, "gate-file", f"added `{a.text.strip()}`"))
    return findings


def skip_findings(added: list[Added]) -> list[Finding]:
    return [
        Finding(a.path, a.line, "test-skip", f"added `{a.text.strip()}`")
        for a in added
        if a.path.endswith("_test.go") and SKIP_CALL.search(a.text)
    ]


def test_funcs(grep: str, rev: bool) -> list[tuple[str, int, str]]:
    funcs = []
    for row in grep.splitlines():
        parts = row.split(":", 3 if rev else 2)[1 if rev else 0:]
        if len(parts) != 3:
            continue
        path, line, text = parts
        funcs.append((path, int(line), re.match(r"func (\w+)", text).group(1)))
    return funcs


def removed_findings(base: list[tuple[str, int, str]], head: list[tuple[str, int, str]]) -> list[Finding]:
    head_names = {name for _, _, name in head}
    base_count = Counter(path for path, _, _ in base)
    head_count = Counter(path for path, _, _ in head)
    return [
        Finding(path, line, "test-removed", f"`{name}` is gone")
        for path, line, name in base
        if name not in head_names and head_count[path] < base_count[path]
    ]


def grep_tests(rev: str | None) -> list[tuple[str, int, str]]:
    args = ["-e", TEST_FUNC, rev] if rev else ["--untracked", "-e", TEST_FUNC]
    return test_funcs(git("grep", "-n", "-E", *args, "--", "*_test.go", ok=(0, 1)), bool(rev))


def diff(*args: str) -> list[Added]:
    return added_lines(git("diff", "-U0", "-M", "--no-color", "--no-ext-diff", *args))


def check(added: list[Added], golangci: str, base_tests, head_tests) -> list[Finding]:
    exclusions = exclusion_lines(golangci) if any(a.path == GOLANGCI for a in added) else set()
    return gate_findings(added, exclusions) + skip_findings(added) + removed_findings(base_tests, head_tests)


def findings(base: str, head: str) -> list[Finding]:
    added = diff(f"{base}...{head}")
    golangci = git("show", f"{head}:{GOLANGCI}") if any(a.path == GOLANGCI for a in added) else ""
    merge_base = git("merge-base", base, head).strip()
    return check(added, golangci, grep_tests(merge_base), grep_tests(head))


def worktree_findings(paths: list[str]) -> list[Finding]:
    added = diff("HEAD", "--", *paths)
    for path in git("ls-files", "--others", "--exclude-standard", "--", *paths).splitlines():
        with open(path, errors="replace") as f:
            added += [Added(path, n, text, True) for n, text in enumerate(f.read().splitlines(), 1)]
    golangci = ""
    if any(a.path == GOLANGCI for a in added):
        with open(GOLANGCI) as f:
            golangci = f.read()
    found = check(added, golangci, grep_tests("HEAD"), grep_tests(None))
    return [f for f in found if not paths or f.path in paths]


def ensure_commit(sha: str) -> None:
    if subprocess.run(["git", "cat-file", "-e", f"{sha}^{{commit}}"], capture_output=True).returncode != 0:
        subprocess.run(["git", "-c", "maintenance.auto=false", "-c", "gc.auto=0", "fetch", "--quiet", "--no-tags", "origin", sha], check=True)


def main() -> int:
    if sys.argv[1:2] == ["--worktree"]:
        found = worktree_findings(sys.argv[2:])
        for f in found:
            print(f)
        return 1 if found else 0
    labels = json.loads(os.environ.get("PR_LABELS") or "[]")
    base, head = os.environ["BASE_SHA"], os.environ["HEAD_SHA"]
    for sha in (base, head):
        ensure_commit(sha)
    found = findings(base, head)
    if not found:
        print("No test gate weakened.")
        return 0
    for f in found:
        print(f)
    if OVERRIDE_LABEL in labels:
        print(f"Allowed by the `{OVERRIDE_LABEL}` label.")
        return 0
    print(f"This PR weakens a test gate. Revert the change, or explain it under \"Reviewer focus\" "
          f"and ask a human reviewer for the `{OVERRIDE_LABEL}` label. Only a human adds it.")
    return 1


if __name__ == "__main__":
    sys.exit(main())
