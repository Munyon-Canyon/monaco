#!/usr/bin/env python3
"""Warn on a pull request that weakens a test gate, unless a human adds `gate-change-approved`.

Diffs BASE_SHA...HEAD_SHA and reports each finding as `path:line: <rule>: <what>`:

- gate-file: an added line in a gate file (coverage.exclude, mutants.allow, a perf baseline,
  a golden file, the `exclusions:` block of apps/backend/.golangci.yml, .swift-format or
  .swiftlint.yml); a new row or a raised count in a row file (ROW_FILES); a lowered value
  in packages/mobile-core/coverage-floor.txt. A newly created gate file is not a finding: it
  adds a gate, it does not loosen one.
- test-skip: an added line in a *_test.go that calls t.Skip, t.Skipf, t.SkipNow or b.Skip*, or
  in a Swift test file that adds XCTSkip, .disabled( or withKnownIssue.
- test-removed: a Go Test, Fuzz or Benchmark function, or a Swift `func test…(` or `@Test`
  function, present at base and absent everywhere at head, unless its base file still has at
  least as many of them (a rename).
- flow-status: a deleted packages/flows/app/<id>.tsv, or its status lowered along
  verified > built > planned. A move to none also counts, unless the same change deletes the
  flow's row from apps/backend/flows.tsv.
- scenario-manifest: an added or removed line inside the generated block of
  scripts/qa/sample-screens.txt, unless the whole block now matches what `monacoctl gen flows`
  writes from the Flow<id>Scenarios.gen.swift enums at head (a regeneration).
- strictness: an added Xcode setting that loosens SWIFT_VERSION (below 6),
  SWIFT_TREAT_WARNINGS_AS_ERRORS (NO) or SWIFT_STRICT_CONCURRENCY (not complete); an added `.v5`
  language mode, unsafeFlags or treatAllWarnings in packages/mobile-core/Package.swift; or a
  `-warnings-as-errors` removed from a call site in a WARNINGS_FLAG_FILES file, unless that
  same line now calls scripts/mobile-core-test.sh, which carries the flag.
- module-graph: an added or removed line inside the `allowedGraph` map of
  scripts/mobile_core_graph_test.go, the table of imports each mobile-core module target may use.

Reads BASE_SHA, HEAD_SHA and PR_LABELS (JSON list of label names) from the environment.
Each finding becomes a GitHub `::warning` annotation plus a line in $GITHUB_STEP_SUMMARY, and
the job still passes: the check is advisory. A crash of the checker itself exits non-zero.
`--worktree [path...]` instead diffs the working tree (untracked files included) against HEAD,
limits findings to the given paths, skips the label check, and exits 1 on any finding. The
Claude Code hook scripts/agent-guard-gates.sh calls it after each edit.
Rules: docs/architecture/ci.md#what-runs-where
"""

from __future__ import annotations

import contextlib
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

SWIFT_TEST_DIRS = ("packages/mobile-core/Tests/", "apps/mobile/MonacoTests/", "apps/mobile/MonacoUITests/")
SWIFT_SKIP = re.compile(r"\bXCTSkip|\.disabled\(|\bwithKnownIssue\b")
SWIFT_TEST_ATTR = re.compile(r"@Test\b")
SWIFT_FUNC = re.compile(r"\bfunc (\w+)\s*[(<]")
SWIFT_CONFIGS = {".swift-format", ".swiftlint.yml"}
ROW_FILES = {
    ".swiftlint-baseline.tsv",
    "packages/mobile-core/Tests/MonacoCoreTests/RepoRulesAllowlist.txt",
    "packages/mobile-core/legacy-baseline.tsv",
    "apps/mobile/MonacoUITests/AccessibilityAuditAllowlist.txt",
    "apps/mobile/MonacoUITests/perf-budgets.tsv",
    "packages/mobile-core/tsan-suppressions.txt",
}
COVERAGE_FLOOR = "packages/mobile-core/coverage-floor.txt"
XCODE_SETTINGS = re.compile(r"^apps/mobile/(Monaco\.xcodeproj/project\.pbxproj|Config/.+\.xcconfig)$")
SETTING = r"\b{}(\[[^\]]*\])?\s*=\s*\"?"
LOOSER_XCODE = (
    re.compile(SETTING.format("SWIFT_VERSION") + r"[0-5](\.\d+)*\b"),
    re.compile(SETTING.format("SWIFT_TREAT_WARNINGS_AS_ERRORS") + r"NO\b"),
    re.compile(SETTING.format("SWIFT_STRICT_CONCURRENCY") + r"(?!complete\b)\w"),
)
PACKAGE_SWIFT = "packages/mobile-core/Package.swift"
LOOSER_MANIFEST = re.compile(r"\.v[45]\b|\bunsafeFlags\b|\btreatAllWarnings\b")
WARNINGS_FLAG = "-warnings-as-errors"
TEST_SCRIPT = "scripts/mobile-core-test.sh"
FLOW_APP = re.compile(r"^packages/flows/app/([^/]+)\.tsv$")
BACKEND_FLOWS = "apps/backend/flows.tsv"
FLOW_RANK = {"planned": 1, "built": 2, "verified": 3}
GRAPH_TEST = "scripts/mobile_core_graph_test.go"
SCENARIO_MANIFEST = "scripts/qa/sample-screens.txt"
SCENARIO_BLOCK = ("# BEGIN generated flow scenarios", "# END generated flow scenarios")
SCENARIO_SWIFT_DIR = "packages/flows/Sources/MonacoFlows/"
SCENARIO_SWIFT = re.compile(r"Flow([0-9]+[a-z]?)Scenarios\.gen\.swift$")
WARNINGS_FLAG_FILES = (
    "Justfile",
    ".github/workflows/ci-mobile-core.yml",
    "apps/backend/cmd/monacoctl/agents/check.go",
    TEST_SCRIPT,
)


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


def diff_lines(diff: str) -> tuple[list[Added], list[Added]]:
    added: list[Added] = []
    removed: list[Added] = []
    path, old_path, new_file, line = "", "", False, 0
    for raw in diff.splitlines():
        if raw.startswith("diff --git "):
            path, old_path, new_file = "", "", False
        elif raw.startswith("new file mode"):
            new_file = True
        elif raw.startswith("rename from "):
            removed.append(Added(raw[len("rename from "):], 1, "", False))
        elif raw.startswith("--- "):
            old_path = raw[6:] if raw.startswith("--- a/") else ""
        elif raw.startswith("+++ "):
            path = raw[6:] if raw.startswith("+++ b/") else ""
        elif m := HUNK.match(raw):
            line = int(m.group(1))
        elif raw.startswith("+") and path:
            added.append(Added(path, line, raw[1:], new_file))
            line += 1
        elif raw.startswith("-") and (path or old_path):
            removed.append(Added(path or old_path, max(line, 1), raw[1:], False))
    return added, removed


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


def graph_lines(source: str) -> set[int]:
    lines: set[int] = set()
    inside = False
    for number, text in enumerate(source.splitlines(), 1):
        if text.startswith("var allowedGraph "):
            inside = True
        elif inside and text.startswith("}"):
            break
        elif inside:
            lines.add(number)
    return lines


def graph_findings(added: list[Added], removed: list[Added], _base, head) -> list[Finding]:
    changed = [c for c in added + removed if c.path == GRAPH_TEST]
    if not changed:
        return []
    table = graph_lines(head(GRAPH_TEST))
    return [
        Finding(c.path, c.line, "module-graph", f"{'added' if c in added else 'removed'} `{c.text.strip()}`")
        for c in changed if c.line in table
    ]


def is_swift_test(path: str) -> bool:
    return path.endswith(".swift") and path.startswith(SWIFT_TEST_DIRS)


def row_key(text: str) -> tuple[str, int] | None:
    text = text.rstrip("\r\n")
    if not text.strip() or text.lstrip().startswith("#"):
        return None
    key, _, last = text.rpartition("\t")
    return (key, int(last)) if key and last.strip().isdigit() else (text, 1)


def row_counts(text: str) -> Counter:
    counts: Counter = Counter()
    for row in filter(None, map(row_key, text.splitlines())):
        counts[row[0]] += row[1]
    return counts


def floors(text: str) -> dict[str, float]:
    values = {}
    for row in text.splitlines():
        parts = row.split()
        if len(parts) == 2:
            with contextlib.suppress(ValueError):
                values[parts[0]] = float(parts[1])
    return values


def gate_findings(added: list[Added], golangci_exclusions: set[int], base, head) -> list[Finding]:
    findings = []
    row_files = {a.path for a in added if a.path in ROW_FILES and not a.new_file}
    base_rows = {path: row_counts(base(path)) for path in row_files}
    head_rows = {path: row_counts(head(path)) for path in row_files}
    base_floor = floors(base(COVERAGE_FLOOR)) if any(a.path == COVERAGE_FLOOR for a in added) else {}
    for a in added:
        gated = (
            a.path in GATE_FILES
            or (NEW_FILE_GATES.search(a.path) and not a.new_file)
            or (a.path == GOLANGCI and a.line in golangci_exclusions)
            or (os.path.basename(a.path) in SWIFT_CONFIGS and not a.new_file)
        )
        if gated:
            findings.append(Finding(a.path, a.line, "gate-file", f"added `{a.text.strip()}`"))
        elif a.path in row_files and (row := row_key(a.text)):
            key, was, now = row[0], base_rows[a.path][row[0]], head_rows[a.path][row[0]]
            if now > was:
                what = f"raised `{' '.join(key.split())}` {was} -> {now}" if was else f"new row `{' '.join(a.text.split())}`"
                findings.append(Finding(a.path, a.line, "gate-file", what))
        elif a.path == COVERAGE_FLOOR:
            for platform, value in floors(a.text).items():
                if value < base_floor.get(platform, value):
                    what = f"lowered `{platform}` {base_floor[platform]:.2f} -> {value:.2f}"
                    findings.append(Finding(a.path, a.line, "gate-file", what))
    return findings


def skip_findings(added: list[Added]) -> list[Finding]:
    return [
        Finding(a.path, a.line, "test-skip", f"added `{a.text.strip()}`")
        for a in added
        if (a.path.endswith("_test.go") and SKIP_CALL.search(a.text))
        or (is_swift_test(a.path) and SWIFT_SKIP.search(a.text))
    ]


def strictness_findings(added: list[Added], removed: list[Added], _base, _head) -> list[Finding]:
    findings = [
        Finding(a.path, a.line, "strictness", f"added `{a.text.strip()}`")
        for a in added
        if (XCODE_SETTINGS.match(a.path) and any(p.search(a.text) for p in LOOSER_XCODE))
        or (a.path == PACKAGE_SWIFT and LOOSER_MANIFEST.search(a.text))
    ]
    for r in removed:
        if r.path not in WARNINGS_FLAG_FILES or WARNINGS_FLAG not in r.text:
            continue
        replacement = [a.text for a in added if a.path == r.path and a.line == r.line]
        if any(WARNINGS_FLAG in text for text in replacement):
            continue
        if r.path != TEST_SCRIPT and any(TEST_SCRIPT in text for text in replacement):
            continue
        findings.append(Finding(r.path, r.line, "strictness", f"removed `{WARNINGS_FLAG}`"))
    return findings


def flow_status(text: str) -> str | None:
    rows = text.splitlines()
    cells = rows[1].split("\t") if len(rows) > 1 else []
    return cells[2] if len(cells) > 2 else None


def has_flow_row(text: str, flow_id: str) -> bool:
    return any(row.split("\t", 1)[0] == flow_id for row in text.splitlines()[1:])


def flow_status_findings(added: list[Added], removed: list[Added], base, head) -> list[Finding]:
    findings = []
    for path in sorted({a.path for a in added + removed if FLOW_APP.match(a.path)}):
        flow_id = FLOW_APP.match(path).group(1)
        was, now = flow_status(base(path)), flow_status(head(path))
        if was is None:
            continue
        if not head(path):
            findings.append(Finding(path, 1, "flow-status", f"deleted the app registry file of flow {flow_id}"))
        elif now == "none" and was != "none":
            if has_flow_row(head(BACKEND_FLOWS), flow_id):
                findings.append(Finding(path, 2, "flow-status", f"lowered flow {flow_id} {was} -> none"))
        elif FLOW_RANK.get(now or "", 0) < FLOW_RANK.get(was, 0):
            findings.append(Finding(path, 2, "flow-status", f"lowered flow {flow_id} {was} -> {now}"))
    return findings


def scenario_block(text: str) -> tuple[range, list[str]]:
    rows = text.splitlines()
    try:
        begin, end = rows.index(SCENARIO_BLOCK[0]), rows.index(SCENARIO_BLOCK[1])
    except ValueError:
        return range(0), []
    lines = [" ".join(row.split()) for row in rows[begin + 1:end] if row.strip() and not row.lstrip().startswith("#")]
    return range(begin + 1, end + 2), lines


def kebab(camel: str) -> str:
    return re.sub(r"[A-Z]", lambda m: "-" + m.group(0).lower(), camel)


def generated_scenario_lines(names: list[str], head) -> list[str]:
    lines = []
    for name in names:
        flow_id = SCENARIO_SWIFT.search(name).group(1)
        cases = re.search(r"\{\n\s*case (.*?)\n\n", head(name), re.S)
        for case in re.findall(r"\w+", cases.group(1) if cases else ""):
            lines.append(f"flow-{flow_id}-{kebab(case)} -MonacoFlow {flow_id} {case}")
    return lines


def scenario_manifest_findings(added: list[Added], removed: list[Added], head, head_ls) -> list[Finding]:
    if not any(a.path == SCENARIO_MANIFEST for a in added + removed):
        return []
    rows, lines = scenario_block(head(SCENARIO_MANIFEST))
    edited = [a for a in added + removed if a.path == SCENARIO_MANIFEST and a.line in rows]
    if not edited:
        return []
    swift = [name for name in head_ls(SCENARIO_SWIFT_DIR) if SCENARIO_SWIFT.search(name)]
    if sorted(lines) == sorted(generated_scenario_lines(swift, head)):
        return []
    return [Finding(e.path, e.line, "scenario-manifest", f"edited scenario manifest line `{e.text.strip()}`")
            for e in edited]


def grep(patterns: list[str], rev: str | None, *pathspecs: str) -> list[tuple[str, int, str]]:
    args = [arg for p in patterns for arg in ("-e", p)]
    args += [rev] if rev else ["--untracked"]
    rows = []
    for row in git("grep", "-n", "-E", *args, "--", *pathspecs, ok=(0, 1)).splitlines():
        parts = row.split(":", 3 if rev else 2)[1 if rev else 0:]
        if len(parts) == 3:
            rows.append((parts[0], int(parts[1]), parts[2]))
    return rows


def go_tests(rows: list[tuple[str, int, str]]) -> list[tuple[str, int, str]]:
    return [(path, line, re.match(r"func (\w+)", text).group(1))
            for path, line, text in rows if path.endswith("_test.go") and re.match(TEST_FUNC, text)]


def swift_tests(rows: list[tuple[str, int, str]]) -> list[tuple[str, int, str]]:
    tests = []
    marked = None
    for path, line, text in rows:
        if not is_swift_test(path):
            continue
        if marked != path:
            marked = None
        if SWIFT_TEST_ATTR.search(text):
            marked = path
        if m := SWIFT_FUNC.search(text):
            if marked or m.group(1).startswith("test"):
                tests.append((path, line, m.group(1)))
            marked = None
    return tests


def removed_findings(base: list[tuple[str, int, str]], head: list[tuple[str, int, str]]) -> list[Finding]:
    head_names = {name for _, _, name in head}
    base_count = Counter(path for path, _, _ in base)
    head_count = Counter(path for path, _, _ in head)
    return [
        Finding(path, line, "test-removed", f"`{name}` is gone")
        for path, line, name in base
        if name not in head_names and head_count[path] < base_count[path]
    ]


def touches_tests(changed: list[Added]) -> bool:
    return any(a.path.endswith("_test.go") or is_swift_test(a.path) for a in changed)


def grep_tests(rev: str | None) -> list[tuple[str, int, str]]:
    rows = grep([TEST_FUNC, "@Test|func [A-Za-z_]"], rev, "*_test.go", *(f"{d}*.swift" for d in SWIFT_TEST_DIRS))
    return go_tests(rows) + swift_tests(rows)


def diff(*args: str) -> tuple[list[Added], list[Added]]:
    return diff_lines(git("diff", "-U0", "-M", "--no-color", "--no-ext-diff", *args))


def show(rev: str):
    return lambda path: git("show", f"{rev}:{path}", ok=(0, 128))


def ls_tree(rev: str):
    return lambda directory: git("ls-tree", "--name-only", rev, "--", directory).splitlines()


def ls_worktree(directory: str) -> list[str]:
    return git("ls-files", "--cached", "--others", "--exclude-standard", "--", directory).splitlines()


def read(path: str) -> str:
    try:
        with open(path, errors="replace") as f:
            return f.read()
    except FileNotFoundError:
        return ""


def check(added: list[Added], removed: list[Added], base, head, head_ls, base_tests, head_tests) -> list[Finding]:
    exclusions = exclusion_lines(head(GOLANGCI)) if any(a.path == GOLANGCI for a in added) else set()
    return (gate_findings(added, exclusions, base, head) + skip_findings(added)
            + removed_findings(base_tests, head_tests) + strictness_findings(added, removed, base, head)
            + flow_status_findings(added, removed, base, head) + graph_findings(added, removed, base, head)
            + scenario_manifest_findings(added, removed, head, head_ls))


def findings(base: str, head: str) -> list[Finding]:
    added, removed = diff(f"{base}...{head}")
    merge_base = git("merge-base", base, head).strip()
    tests = touches_tests(added + removed)
    return check(added, removed, show(merge_base), show(head), ls_tree(head),
                 grep_tests(merge_base) if tests else [], grep_tests(head) if tests else [])


def worktree_findings(paths: list[str]) -> list[Finding]:
    added, removed = diff("HEAD", "--", *paths)
    for path in git("ls-files", "--others", "--exclude-standard", "--", *paths).splitlines():
        added += [Added(path, n, text, True) for n, text in enumerate(read(path).splitlines(), 1)]
    tests = touches_tests(added + removed)
    found = check(added, removed, show("HEAD"), read, ls_worktree,
                  grep_tests("HEAD") if tests else [], grep_tests(None) if tests else [])
    return [f for f in found if not paths or f.path in paths]


def ensure_commits(*shas: str) -> None:
    query = "".join(f"{sha}^{{commit}}\n" for sha in shas)
    found = subprocess.run(["git", "cat-file", "--batch-check"], input=query, capture_output=True, text=True).stdout
    for sha, row in zip(shas, found.splitlines()):
        if row.endswith(" missing"):
            subprocess.run(["git", "-c", "maintenance.auto=false", "-c", "gc.auto=0", "fetch", "--quiet", "--no-tags", "origin", sha], check=True)


def escape(text: str, prop: bool = False) -> str:
    text = text.replace("%", "%25").replace("\r", "%0D").replace("\n", "%0A")
    return text.replace(":", "%3A").replace(",", "%2C") if prop else text


def annotation(f: Finding) -> str:
    return (f"::warning file={escape(f.path, True)},line={f.line},title=Test gate weakened::"
            + escape(f"{f.rule}: {f.what}. The `{OVERRIDE_LABEL}` label silences this."))


def step_summary(found: list[Finding]) -> str:
    rows = "".join(f"- `{f.path}:{f.line}` {f.rule}: {f.what}\n" for f in found)
    return (f"### This PR weakens a test gate\n\n{rows}\nRevert the change, or explain it under "
            f"\"Reviewer focus\" and ask a human reviewer for the `{OVERRIDE_LABEL}` label, "
            "which silences this warning. Only a human adds it. The check never blocks the merge.\n")


def main() -> int:
    if sys.argv[1:2] == ["--worktree"]:
        found = worktree_findings(sys.argv[2:])
        for f in found:
            print(f)
        return 1 if found else 0
    labels = json.loads(os.environ.get("PR_LABELS") or "[]")
    base, head = os.environ["BASE_SHA"], os.environ["HEAD_SHA"]
    ensure_commits(base, head)
    found = findings(base, head)
    if not found:
        print("No test gate weakened.")
        return 0
    if OVERRIDE_LABEL in labels:
        for f in found:
            print(f)
        print(f"Allowed by the `{OVERRIDE_LABEL}` label.")
        return 0
    for f in found:
        print(annotation(f))
    summary = os.environ.get("GITHUB_STEP_SUMMARY")
    if summary:
        with open(summary, "a") as out:
            out.write(step_summary(found))
    return 0


if __name__ == "__main__":
    sys.exit(main())
