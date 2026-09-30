import contextlib
import importlib.util
import io
import os
import pathlib
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

spec = importlib.util.spec_from_file_location(
    "check_gate_changes", pathlib.Path(__file__).with_name("check-gate-changes.py")
)
check = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = check
spec.loader.exec_module(check)

GOLANGCI = """version: "2"
linters:
  exclusions:
    rules:
      - path: _test\\.go$
        linters: [funlen]
  settings:
    funlen: { lines: 80 }
"""

TESTS = """package fund

import "testing"

func TestDeposit(t *testing.T) {
	if 1+1 != 2 {
		t.Fatal("math")
	}
}

func FuzzAmount(f *testing.F) {}

func testServer() {}
"""

SWIFT_TESTS = """import Testing
import XCTest

final class FundTests: XCTestCase {
    func testDeposit() {
        XCTAssertEqual(1 + 1, 2)
    }

    func testWithdraw() {}
}

@Suite struct AmountTests {
    @Test func decodes() {}

    @Test("encodes cents")
    func encodes() {}

    func helper() {}
}
"""

SWIFT_FILE = "packages/mobile-core/Tests/MonacoCoreTests/FundTests.swift"
APP_TESTS = "apps/mobile/MonacoTests/CabalTests.swift"
UI_TESTS = "apps/mobile/MonacoUITests/LoginUITests.swift"
XCCONFIG = "apps/mobile/Config/Monaco.xcconfig"
PBXPROJ = "apps/mobile/Monaco.xcodeproj/project.pbxproj"
ROWS = {
    ".swiftlint-baseline.tsv": "force_unwrapping\tapps/mobile/Monaco/A.swift\t3\n",
    "packages/mobile-core/Tests/MonacoCoreTests/RepoRulesAllowlist.txt": "urlsession\tapps/mobile/Monaco/B.swift\t2\n",
    "packages/mobile-core/legacy-baseline.tsv": "# keep the smaller count\nlines\tapps/mobile/Monaco/API/C.swift\t120\n",
    "apps/mobile/MonacoUITests/AccessibilityAuditAllowlist.txt": "CabalsTabSampleUITests\tcontrast\tbutton.join\n",
    "packages/mobile-core/tsan-suppressions.txt": "race:libdispatch\n",
}

BASE = {
    "apps/backend/coverage.exclude": "cmd/api/main.go\n",
    "apps/backend/mutants.allow": "",
    "apps/backend/.golangci.yml": GOLANGCI,
    "apps/backend/internal/db/testdata/perf/baseline.json": '{"allocs": 10}\n',
    "apps/backend/internal/events/testdata/golden/system.pinged.v1.json": '{"type": "system.pinged"}\n',
    "apps/backend/internal/app/fund_test.go": TESTS,
    SWIFT_FILE: SWIFT_TESTS,
    APP_TESTS: "import XCTest\n\nfinal class CabalTests: XCTestCase {\n    func testJoin() {}\n}\n",
    UI_TESTS: "import XCTest\n\nfinal class LoginUITests: XCTestCase {\n    func testLogin() {}\n}\n",
    "packages/mobile-core/Sources/MonacoCore/Fund.swift": "struct Fund {}\n",
    ".swiftlint.yml": "opt_in_rules:\n  - force_unwrapping\n",
    ".swift-format": '{"version": 1}\n',
    "packages/mobile-core/coverage-floor.txt": "darwin 89.24\nlinux 88.10\n",
    XCCONFIG: "SWIFT_VERSION = 5.0\n",
    PBXPROJ: "\t\t\t\tSWIFT_VERSION = 5.0;\n",
    "packages/mobile-core/Package.swift": "let package = Package(name: \"MonacoCore\")\n",
    "Justfile": "test:\n    cd packages/mobile-core && swift test -Xswiftc -warnings-as-errors\n",
    **ROWS,
}


TEMPLATE = tempfile.TemporaryDirectory()


class Repo:
    def __init__(self, root: pathlib.Path, name="base", extra=None):
        self.root = pathlib.Path(TEMPLATE.name) / name
        if not self.root.exists():
            self.root.mkdir()
            self.git("init", "-q", "-b", "main")
            # git commit spawns a detached `git maintenance run --auto` that can write
            # .git/objects while TemporaryDirectory.cleanup removes the repo.
            self.git("config", "maintenance.auto", "false")
            self.git("config", "gc.auto", "0")
            self.git("config", "gc.autoDetach", "false")
            self.git("config", "user.email", "t@example.com")
            self.git("config", "user.name", "t")
            self.commit(BASE)
            if extra:
                extra(self)
        shutil.copytree(self.root, root, dirs_exist_ok=True)
        self.root = root
        self.base = self.sha()

    def git(self, *args):
        return subprocess.run(["git", *args], cwd=self.root, check=True, capture_output=True, text=True).stdout

    def sha(self):
        return (self.root / ".git" / "refs" / "heads" / "main").read_text().strip()

    def write(self, files):
        for path, text in files.items():
            p = self.root / path
            if text is None:
                p.unlink()
                continue
            p.parent.mkdir(parents=True, exist_ok=True)
            p.write_text(text)

    def commit(self, files):
        self.write(files)
        self.git("add", "-A")
        self.git("commit", "-q", "--allow-empty", "-m", "change")
        return self.sha()


class CheckTest(unittest.TestCase):
    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        cwd = os.getcwd()
        self.addCleanup(os.chdir, cwd)
        os.chdir(tmp.name)
        self.repo = Repo(pathlib.Path(tmp.name))

    def run_check(self, files, labels="[]"):
        head = self.repo.commit(files)
        summary = self.repo.root / "summary.md"
        summary.write_text("")
        env = {"BASE_SHA": self.repo.base, "HEAD_SHA": head, "PR_LABELS": labels,
               "GITHUB_STEP_SUMMARY": str(summary)}
        out = io.StringIO()
        with mock.patch.dict(os.environ, env), contextlib.redirect_stdout(out):
            code = check.main()
        self.summary = summary.read_text()
        return code, out.getvalue()

    def assert_flags(self, files, *findings):
        code, out = self.run_check(files)
        self.assertEqual(code, 0, out)
        for finding in findings:
            path, line, what = finding.split(":", 2)
            self.assertIn(f"::warning file={path},line={line},title=Test gate weakened::{what.strip().replace('%', '%25')}", out)
            self.assertIn(f"- `{path}:{line}` {what.strip()}", self.summary)
        self.assertIn("gate-change-approved` label silences this.", out)
        self.assertIn("gate-change-approved", self.summary)
        self.assertIn("Reviewer focus", self.summary)

    def assert_clean(self, files):
        code, out = self.run_check(files)
        self.assertEqual(code, 0, out)
        self.assertIn("No test gate weakened.", out)

    def test_clean_diff_passes(self):
        code, out = self.run_check({
            "apps/backend/internal/app/fund.go": "package fund\n",
            "apps/backend/internal/app/fund_test.go": TESTS + "\nfunc TestWithdraw(t *testing.T) {}\n",
            "apps/backend/internal/app/new_test.go": "package fund\n\nfunc TestNew(t *testing.T) {}\n",
        })
        self.assertEqual(code, 0, out)
        self.assertIn("No test gate weakened.", out)
        self.assertNotIn("::warning", out)
        self.assertEqual(self.summary, "")

    def test_coverage_exclude_line(self):
        self.assert_flags(
            {"apps/backend/coverage.exclude": "cmd/api/main.go\ninternal/app/fund.go\n"},
            "apps/backend/coverage.exclude:2: gate-file: added `internal/app/fund.go`",
        )

    def test_mutants_allow_line(self):
        self.assert_flags(
            {"apps/backend/mutants.allow": "internal/app/fund.go:12 CONDITIONALS_BOUNDARY\n"},
            "apps/backend/mutants.allow:1: gate-file: added `internal/app/fund.go:12 CONDITIONALS_BOUNDARY`",
        )

    def test_raised_perf_baseline(self):
        self.assert_flags(
            {"apps/backend/internal/db/testdata/perf/baseline.json": '{"allocs": 40}\n'},
            'apps/backend/internal/db/testdata/perf/baseline.json:1: gate-file: added `{"allocs": 40}`',
        )

    def test_edited_golden_file(self):
        self.assert_flags(
            {"apps/backend/internal/events/testdata/golden/system.pinged.v1.json": '{"type": "x"}\n'},
            "apps/backend/internal/events/testdata/golden/system.pinged.v1.json:1: gate-file:",
        )

    def test_new_golden_and_baseline_files_pass(self):
        code, out = self.run_check({
            "apps/backend/internal/events/testdata/golden/fund.created.v1.json": "{}\n",
            "apps/backend/internal/bus/testdata/perf/baseline.json": '{"allocs": 3}\n',
        })
        self.assertEqual(code, 0, out)

    def test_golangci_exclusion_rule(self):
        self.assert_flags(
            {"apps/backend/.golangci.yml": GOLANGCI.replace(
                "  settings:", "      - path: internal/app/\n        linters: [err113]\n  settings:")},
            "apps/backend/.golangci.yml:7: gate-file: added `- path: internal/app/`",
        )

    def test_golangci_change_outside_exclusions_passes(self):
        code, out = self.run_check({"apps/backend/.golangci.yml": GOLANGCI.replace("80", "60")})
        self.assertEqual(code, 0, out)

    def test_skip_calls(self):
        for call in ('t.Skip("flaky")', 't.Skipf("flaky %d", 1)', "t.SkipNow()", 'b.Skip("slow")'):
            with self.subTest(call=call):
                self.repo.git("reset", "-q", "--hard", self.repo.base)
                self.assert_flags(
                    {"apps/backend/internal/app/fund_test.go": TESTS.replace(
                        "\tif 1+1", f"\t{call}\n\tif 1+1")},
                    f"apps/backend/internal/app/fund_test.go:6: test-skip: added `{call}`",
                )

    def test_deleted_test_function(self):
        self.assert_flags(
            {"apps/backend/internal/app/fund_test.go": TESTS.replace("func FuzzAmount(f *testing.F) {}\n", "")},
            "apps/backend/internal/app/fund_test.go:11: test-removed: `FuzzAmount` is gone",
        )

    def test_deleted_test_file(self):
        self.assert_flags(
            {"apps/backend/internal/app/fund_test.go": None},
            "apps/backend/internal/app/fund_test.go:5: test-removed: `TestDeposit` is gone",
        )

    def test_renamed_test_function_passes(self):
        code, out = self.run_check(
            {"apps/backend/internal/app/fund_test.go": TESTS.replace("TestDeposit", "TestDepositCredits")})
        self.assertEqual(code, 0, out)

    def test_deleted_go_helper_passes(self):
        self.assert_clean({"apps/backend/internal/app/fund_test.go": TESTS.replace("\nfunc testServer() {}\n", "")})

    def test_moved_test_function_passes(self):
        code, out = self.run_check({
            "apps/backend/internal/app/fund_test.go": TESTS.replace("func FuzzAmount(f *testing.F) {}\n", ""),
            "apps/backend/internal/app/amount_test.go": "package fund\n\nfunc FuzzAmount(f *testing.F) {}\n",
        })
        self.assertEqual(code, 0, out)

    def test_swift_skips(self):
        self.assert_flags({
            SWIFT_FILE: SWIFT_TESTS.replace("        XCTAssertEqual", "        try XCTSkipIf(true)\n        XCTAssertEqual"),
            APP_TESTS: BASE[APP_TESTS] + '\n@Test(.disabled("flaky")) func joins() {}\n',
            UI_TESTS: BASE[UI_TESTS].replace("func testLogin() {}", "func testLogin() {\n        withKnownIssue { fail() }\n    }"),
        },
            f"{SWIFT_FILE}:6: test-skip: added `try XCTSkipIf(true)`",
            f'{APP_TESTS}:7: test-skip: added `@Test(.disabled("flaky")) func joins() {{}}`',
            f"{UI_TESTS}:5: test-skip: added `withKnownIssue {{ fail() }}`",
        )

    def test_deleted_swift_tests(self):
        self.assert_flags({
            SWIFT_FILE: SWIFT_TESTS.replace("    func testWithdraw() {}\n", "")
            .replace('    @Test("encodes cents")\n    func encodes() {}\n', ""),
            APP_TESTS: None,
        },
            f"{SWIFT_FILE}:9: test-removed: `testWithdraw` is gone",
            f"{SWIFT_FILE}:16: test-removed: `encodes` is gone",
            f"{APP_TESTS}:4: test-removed: `testJoin` is gone",
        )

    def test_deleted_swift_helper_and_renamed_test_pass(self):
        self.assert_clean(
            {SWIFT_FILE: SWIFT_TESTS.replace("    func helper() {}\n", "").replace("testDeposit", "testDepositCredits")})

    def test_moved_swift_test_passes(self):
        self.assert_clean({
            SWIFT_FILE: SWIFT_TESTS.replace("    @Test func decodes() {}\n", ""),
            "packages/mobile-core/Tests/MonacoCoreTests/DecodeTests.swift": "@Test func decodes() {}\n",
        })

    def test_swift_lint_config_lines(self):
        self.assert_flags({
            ".swiftlint.yml": BASE[".swiftlint.yml"] + "disabled_rules:\n  - force_cast\n",
            ".swift-format": '{"version": 1, "lineLength": 200}\n',
        },
            ".swiftlint.yml:3: gate-file: added `disabled_rules:`",
            '.swift-format:1: gate-file: added `{"version": 1, "lineLength": 200}`',
        )

    def test_row_file_new_rows(self):
        files, findings = {}, []
        for path, text in ROWS.items():
            files[path] = text + "extra\tapps/mobile/Monaco/Z.swift\t1\n"
            findings.append(f"{path}:{len(text.splitlines()) + 1}: gate-file: new row `extra apps/mobile/Monaco/Z.swift 1`")
        self.assert_flags(files, *findings)

    def test_row_file_raised_counts(self):
        files, findings = {}, []
        for path, text in ROWS.items():
            key, _, count = text.splitlines()[-1].rpartition("\t")
            if count.isdigit():
                files[path] = text.replace(f"\t{count}\n", f"\t{int(count) + 1}\n")
                findings.append(f"{path}:{len(text.splitlines())}: gate-file: raised `{' '.join(key.split())}` "
                                f"{count} -> {int(count) + 1}")
        self.assertEqual(len(findings), 3)
        self.assert_flags(files, *findings)

    def test_row_file_new_row_among_unchanged_rows(self):
        path = "packages/mobile-core/legacy-baseline.tsv"
        code, out = self.run_check({path: "# keep the smaller count\nlines\tapps/mobile/Monaco/API/B.swift\t4\n"
                                          "lines\tapps/mobile/Monaco/API/C.swift\t120\n"})
        self.assertIn("new row `lines apps/mobile/Monaco/API/B.swift 4`", out)
        self.assertNotIn("C.swift", out)

    def test_coverage_floor(self):
        self.assert_flags(
            {"packages/mobile-core/coverage-floor.txt": "darwin 88.00\nlinux 88.10\n"},
            "packages/mobile-core/coverage-floor.txt:1: gate-file: lowered `darwin` 89.24 -> 88.00",
        )
        self.assertNotIn("linux", self.summary)

    def test_looser_swift_settings(self):
        xcconfig = ["SWIFT_TREAT_WARNINGS_AS_ERRORS = NO", "SWIFT_STRICT_CONCURRENCY = targeted",
                    "SWIFT_VERSION[sdk=iphoneos*] = 5"]
        pbxproj = ["\t\t\t\tSWIFT_VERSION = 4.2;", '\t\t\t\tSWIFT_STRICT_CONCURRENCY = "minimal";']
        manifest = ["swiftLanguageModes: [.v5],", '.unsafeFlags(["-suppress-warnings"]),', ".treatAllWarnings(as: .warning),"]
        env = "apps/mobile/Config/Environments/Debug.xcconfig"
        package = "packages/mobile-core/Package.swift"
        findings = [f"{XCCONFIG}:{n}: strictness: added `{s}`" for n, s in enumerate(xcconfig, 2)]
        findings += [f"{PBXPROJ}:{n}: strictness: added `{s.strip()}`" for n, s in enumerate(pbxproj, 2)]
        findings += [f"{package}:{n}: strictness: added `{s}`" for n, s in enumerate(manifest, 2)]
        findings.append(f"{env}:1: strictness: added `SWIFT_TREAT_WARNINGS_AS_ERRORS=NO`")
        self.assert_flags({
            XCCONFIG: BASE[XCCONFIG] + "".join(f"{s}\n" for s in xcconfig),
            PBXPROJ: BASE[PBXPROJ] + "".join(f"{s}\n" for s in pbxproj),
            package: BASE[package] + "".join(f"{s}\n" for s in manifest),
            env: "SWIFT_TREAT_WARNINGS_AS_ERRORS=NO\n",
        }, *findings)

    def test_changes_that_keep_or_tighten_swift_gates_pass(self):
        self.assert_clean({
            "packages/mobile-core/Sources/MonacoCore/Fund.swift": "struct Fund { let disabled = XCTSkip.self }\n",
            "apps/mobile/.swiftlint.yml": "opt_in_rules:\n  - force_try\n",
            ".swiftlint-baseline.tsv": ROWS[".swiftlint-baseline.tsv"].replace("\t3\n", "\t2\n"),
            "packages/mobile-core/coverage-floor.txt": "darwin 89.80\nlinux 88.10\n",
            XCCONFIG: "SWIFT_VERSION = 6.0\nSWIFT_TREAT_WARNINGS_AS_ERRORS = YES\nSWIFT_STRICT_CONCURRENCY = complete\n",
            PBXPROJ: "\t\t\t\tSWIFT_VERSION = 6.0;\n\t\t\t\tSWIFT_STRICT_CONCURRENCY = complete;\n",
            "Justfile": "test:\n    scripts/mobile-core-test.sh\n",
            "scripts/mobile-core-test.sh": "swift test -Xswiftc -warnings-as-errors --enable-code-coverage\n",
        })

    def test_pure_rename_of_a_test_file_is_test_removed(self):
        go = "apps/backend/internal/app/fund_test.go"
        self.assert_flags({
            go: None,
            go + ".off": BASE[go],
            SWIFT_FILE: None,
            "packages/mobile-core/Disabled/FundTests.swift": BASE[SWIFT_FILE],
        },
            f"{go}:5: test-removed: `TestDeposit` is gone",
            f"{go}:11: test-removed: `FuzzAmount` is gone",
            f"{SWIFT_FILE}:5: test-removed: `testDeposit` is gone",
            f"{SWIFT_FILE}:9: test-removed: `testWithdraw` is gone",
            f"{SWIFT_FILE}:13: test-removed: `decodes` is gone",
            f"{SWIFT_FILE}:16: test-removed: `encodes` is gone",
        )

    def test_removed_warnings_as_errors(self):
        self.assert_flags(
            {"Justfile": "test:\n    cd packages/mobile-core && swift test\n"},
            "Justfile:2: strictness: removed `-warnings-as-errors`",
        )

    def test_warnings_as_errors_exemption_is_per_call_site(self):
        yml = """name: CI mobile-core

# Called from ci-jobs.yml's mobile-core job. Kept in its own file so the `plan` job's
# `mobile-core` path filter can key on this file instead of every `ci*.yml`, which every
# backend PR touches. docs/architecture/ci.md#triggers
on:
  workflow_call:

permissions:
  contents: read

jobs:
  mobile-core:
    # The macOS job below compiles the Darwin-only code this skips.
    name: Swift (mobile-core, Linux)
    runs-on: ubuntu-latest
    timeout-minutes: 10
    container: swift:6.3-noble
    steps:
      - uses: actions/checkout@v4

      - name: Test mobile-core
        working-directory: packages/mobile-core
        run: swift test -Xswiftc -warnings-as-errors

  mobile-core-macos:
    # Xcode builds MonacoCore with -suppress-warnings, so only a SwiftPM build fails on a warning
    # in Darwin-only code. docs/architecture/ci.md#what-runs-where
    name: Swift (mobile-core, macOS)
    runs-on: macos-15
    timeout-minutes: 15
    steps:
      - uses: actions/checkout@v4

      - name: Select Xcode
        run: scripts/ci/select-xcode.sh

      - name: Build mobile-core and its tests
        working-directory: packages/mobile-core
        run: swift build --build-tests -Xswiftc -warnings-as-errors
"""
        path = ".github/workflows/ci-mobile-core.yml"
        linux = "        run: swift test -Xswiftc -warnings-as-errors\n"
        macos = "        run: swift build --build-tests -Xswiftc -warnings-as-errors\n"
        self.repo.base = self.repo.commit({path: yml})
        script_only = yml.replace(linux, "        run: scripts/mobile-core-test.sh\n", 1)
        self.assert_clean({path: script_only})
        self.repo.git("reset", "-q", "--hard", self.repo.base)
        both = script_only.replace(macos, "        run: swift build --build-tests\n", 1)
        code, out = self.run_check({path: both})
        self.assertEqual(code, 0, out)
        self.assertIn(
            f"::warning file={path},line=40,title=Test gate weakened::strictness: removed `-warnings-as-errors`",
            out,
        )
        self.assertNotIn(f"file={path},line=24,", out)
        self.assertIn(f"- `{path}:40` strictness: removed `-warnings-as-errors`", self.summary)
        self.assertNotIn(f"`{path}:24`", self.summary)

    def test_warnings_as_errors_removed_from_test_script(self):
        self.repo.base = self.repo.commit({"scripts/mobile-core-test.sh": "swift test -Xswiftc -warnings-as-errors\n"})
        self.assert_flags(
            {"scripts/mobile-core-test.sh": "swift test\n"},
            "scripts/mobile-core-test.sh:1: strictness: removed `-warnings-as-errors`",
        )

    def test_label_overrides_every_rule(self):
        code, out = self.run_check({
            "apps/backend/mutants.allow": "internal/app/fund.go:12 CONDITIONALS_BOUNDARY\n",
            "apps/backend/internal/app/fund_test.go": TESTS.replace("\tif 1+1", '\tt.Skip("flaky")\n\tif 1+1')
            .replace("func FuzzAmount(f *testing.F) {}\n", ""),
        }, labels='["large-pr", "gate-change-approved"]')
        self.assertEqual(code, 0, out)
        for rule in ("gate-file", "test-skip", "test-removed"):
            self.assertIn(rule, out)
        self.assertIn("Allowed by the `gate-change-approved` label.", out)
        self.assertNotIn("::warning", out)
        self.assertEqual(self.summary, "")

    def test_other_labels_do_not_override(self):
        code, out = self.run_check({"apps/backend/mutants.allow": "x\n"}, labels='["large-pr"]')
        self.assertEqual(code, 0)
        self.assertIn("::warning file=apps/backend/mutants.allow,line=1,", out)

    def test_annotation_escapes_workflow_command_characters(self):
        finding = check.Finding("a,b:c.go", 3, "gate-file", "added `50%\nx`")
        self.assertEqual(
            check.annotation(finding),
            "::warning file=a%2Cb%3Ac.go,line=3,title=Test gate weakened::"
            "gate-file: added `50%25%0Ax`. The `gate-change-approved` label silences this.",
        )

    def test_checker_crash_fails_the_job(self):
        env = {**os.environ, "BASE_SHA": self.repo.base, "HEAD_SHA": "0" * 40, "PR_LABELS": "[]"}
        env.pop("GITHUB_STEP_SUMMARY", None)
        result = subprocess.run(
            [sys.executable, str(pathlib.Path(__file__).with_name("check-gate-changes.py"))],
            cwd=self.repo.root, env=env, capture_output=True, text=True,
        )
        self.assertNotEqual(result.returncode, 0, result.stdout)


def add_scripts(repo):
    scripts = pathlib.Path(__file__).parent
    for name in ("check-gate-changes.py", "agent-guard-gates.sh"):
        target = repo.root / "scripts" / name
        target.parent.mkdir(exist_ok=True)
        target.write_text((scripts / name).read_text())
        target.chmod(0o755)
    repo.commit({})


class WorktreeTest(unittest.TestCase):
    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.root = pathlib.Path(tmp.name)
        self.repo = Repo(self.root, "hooked", add_scripts)

    def hook(self, path):
        stdin = '{"hook_event_name": "PostToolUse", "tool_input": {"file_path": "%s"}}' % (self.root / path)
        return subprocess.run(
            [str(self.root / "scripts" / "agent-guard-gates.sh")],
            input=stdin, capture_output=True, text=True, cwd=self.root,
        )

    def test_added_skip_returns_exit_2_with_the_finding(self):
        path = "apps/backend/internal/app/fund_test.go"
        self.repo.write({path: TESTS.replace("\tif 1+1", '\tt.Skip("flaky")\n\tif 1+1')})
        result = self.hook(path)
        self.assertEqual(result.returncode, 2, result.stderr)
        self.assertEqual(
            result.stderr,
            f'{path}:6: test-skip: added `t.Skip("flaky")`. This weakens a regression gate. Revert it unless '
            "the user asked for it in this session; if they did, say so under Reviewer focus in the PR.\n",
        )

    def test_new_test_function_and_new_test_file_pass_silently(self):
        path = "apps/backend/internal/app/fund_test.go"
        new = "apps/backend/internal/app/new_test.go"
        self.repo.write({
            path: TESTS + "\nfunc TestWithdraw(t *testing.T) {}\n",
            new: "package fund\n\nfunc TestNew(t *testing.T) {}\n",
        })
        for p in (path, new):
            result = self.hook(p)
            self.assertEqual((result.returncode, result.stderr), (0, ""))

    def test_untracked_test_file_with_skip_is_flagged(self):
        new = "apps/backend/internal/app/new_test.go"
        self.repo.write({new: 'package fund\n\nfunc TestNew(t *testing.T) {\n\tt.SkipNow()\n}\n'})
        result = self.hook(new)
        self.assertEqual(result.returncode, 2)
        self.assertIn(f"{new}:4: test-skip", result.stderr)

    def test_deleted_function_and_gate_file_line(self):
        self.repo.write({
            "apps/backend/internal/app/fund_test.go": TESTS.replace("func FuzzAmount(f *testing.F) {}\n", ""),
            "apps/backend/mutants.allow": "internal/app/fund.go:1 X\n",
        })
        self.assertIn("test-removed: `FuzzAmount` is gone", self.hook("apps/backend/internal/app/fund_test.go").stderr)
        self.assertIn("mutants.allow:1: gate-file", self.hook("apps/backend/mutants.allow").stderr)

    def test_swift_gate_edits_are_flagged(self):
        cases = {
            SWIFT_FILE: (SWIFT_TESTS.replace("        XCTAssertEqual", "        throw XCTSkip()\n        XCTAssertEqual")
                         .replace("    @Test func decodes() {}\n", ""), ("test-skip", "test-removed: `decodes` is gone")),
            ".swiftlint-baseline.tsv": (ROWS[".swiftlint-baseline.tsv"].replace("\t3\n", "\t4\n"), ("gate-file: raised",)),
            ".swiftlint.yml": ("opt_in_rules:\n  - force_unwrapping\n  - x\n", ("gate-file: added `- x`",)),
            "packages/mobile-core/coverage-floor.txt": ("darwin 80.00\nlinux 88.10\n", ("gate-file: lowered",)),
            XCCONFIG: ("SWIFT_VERSION = 5.0\nSWIFT_TREAT_WARNINGS_AS_ERRORS = NO\n", ("strictness",)),
            "packages/mobile-core/Package.swift": ("unsafeFlags\n", ("strictness",)),
            "Justfile": ("test:\n    swift test\n", ("strictness: removed",)),
        }
        self.repo.write({path: text for path, (text, _) in cases.items()})
        for path, (_, findings) in cases.items():
            with self.subTest(path=path):
                result = self.hook(path)
                self.assertEqual(result.returncode, 2, result.stderr)
                for finding in findings:
                    self.assertIn(finding, result.stderr)

    def test_lowered_swift_row_and_newer_swift_version_pass_silently(self):
        for path, text in (
            (".swiftlint-baseline.tsv", ROWS[".swiftlint-baseline.tsv"].replace("\t3\n", "\t1\n")),
            (XCCONFIG, "SWIFT_VERSION = 6.0\n"),
            (APP_TESTS, BASE[APP_TESTS] + "\nfinal class MoreTests: XCTestCase {\n    func testMore() {}\n}\n"),
        ):
            with self.subTest(path=path):
                self.repo.write({path: text})
                result = self.hook(path)
                self.assertEqual((result.returncode, result.stderr), (0, ""))

    def test_findings_are_limited_to_the_edited_path(self):
        self.repo.write({"apps/backend/mutants.allow": "internal/app/fund.go:1 X\n"})
        result = self.hook("apps/backend/internal/app/fund_test.go")
        self.assertEqual((result.returncode, result.stderr), (0, ""))

    def test_non_gate_path_is_ignored(self):
        self.repo.write({"apps/backend/internal/app/fund.go": "package fund\n"})
        self.assertEqual(self.hook("apps/backend/internal/app/fund.go").returncode, 0)
        self.repo.write({"packages/mobile-core/Sources/MonacoCore/Fund.swift": "XCTSkip\n"})
        self.assertEqual(self.hook("packages/mobile-core/Sources/MonacoCore/Fund.swift").returncode, 0)


class ExclusionLinesTest(unittest.TestCase):
    def test_block_ends_at_sibling_key(self):
        self.assertEqual(check.exclusion_lines(GOLANGCI), {4, 5, 6})


if __name__ == "__main__":
    unittest.main()
