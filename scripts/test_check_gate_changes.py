import contextlib
import importlib.util
import io
import os
import pathlib
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
"""

BASE = {
    "apps/backend/coverage.exclude": "cmd/api/main.go\n",
    "apps/backend/mutants.allow": "",
    "apps/backend/.golangci.yml": GOLANGCI,
    "apps/backend/internal/db/testdata/perf/baseline.json": '{"allocs": 10}\n',
    "apps/backend/internal/events/testdata/golden/system.pinged.v1.json": '{"type": "system.pinged"}\n',
    "apps/backend/internal/app/fund_test.go": TESTS,
}


class Repo:
    def __init__(self, root: pathlib.Path):
        self.root = root
        self.git("init", "-q", "-b", "main")
        # git commit spawns a detached `git maintenance run --auto` that can write
        # .git/objects while TemporaryDirectory.cleanup removes the repo.
        self.git("config", "maintenance.auto", "false")
        self.git("config", "gc.auto", "0")
        self.git("config", "gc.autoDetach", "false")
        self.git("config", "user.email", "t@example.com")
        self.git("config", "user.name", "t")
        self.commit(BASE)
        self.base = self.sha()

    def git(self, *args):
        return subprocess.run(["git", *args], cwd=self.root, check=True, capture_output=True, text=True).stdout

    def sha(self):
        return self.git("rev-parse", "HEAD").strip()

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
        env = {"BASE_SHA": self.repo.base, "HEAD_SHA": head, "PR_LABELS": labels}
        out = io.StringIO()
        with mock.patch.dict(os.environ, env), contextlib.redirect_stdout(out):
            code = check.main()
        return code, out.getvalue()

    def assert_flags(self, files, finding):
        code, out = self.run_check(files)
        self.assertEqual(code, 1, out)
        self.assertIn(finding, out)
        self.assertIn("gate-change-approved", out)
        self.assertIn("Reviewer focus", out)

    def test_clean_diff_passes(self):
        code, out = self.run_check({
            "apps/backend/internal/app/fund.go": "package fund\n",
            "apps/backend/internal/app/fund_test.go": TESTS + "\nfunc TestWithdraw(t *testing.T) {}\n",
            "apps/backend/internal/app/new_test.go": "package fund\n\nfunc TestNew(t *testing.T) {}\n",
        })
        self.assertEqual(code, 0, out)
        self.assertIn("No test gate weakened.", out)

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

    def test_moved_test_function_passes(self):
        code, out = self.run_check({
            "apps/backend/internal/app/fund_test.go": TESTS.replace("func FuzzAmount(f *testing.F) {}\n", ""),
            "apps/backend/internal/app/amount_test.go": "package fund\n\nfunc FuzzAmount(f *testing.F) {}\n",
        })
        self.assertEqual(code, 0, out)

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

    def test_other_labels_do_not_override(self):
        code, _ = self.run_check({"apps/backend/mutants.allow": "x\n"}, labels='["large-pr"]')
        self.assertEqual(code, 1)


class WorktreeTest(unittest.TestCase):
    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.root = pathlib.Path(tmp.name)
        self.repo = Repo(self.root)
        scripts = pathlib.Path(__file__).parent
        for name in ("check-gate-changes.py", "agent-guard-gates.sh"):
            target = self.root / "scripts" / name
            target.parent.mkdir(exist_ok=True)
            target.write_text((scripts / name).read_text())
            target.chmod(0o755)
        self.repo.commit({})

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

    def test_findings_are_limited_to_the_edited_path(self):
        self.repo.write({"apps/backend/mutants.allow": "internal/app/fund.go:1 X\n"})
        result = self.hook("apps/backend/internal/app/fund_test.go")
        self.assertEqual((result.returncode, result.stderr), (0, ""))

    def test_non_gate_path_is_ignored(self):
        self.repo.write({"apps/backend/internal/app/fund.go": "package fund\n"})
        self.assertEqual(self.hook("apps/backend/internal/app/fund.go").returncode, 0)


class ExclusionLinesTest(unittest.TestCase):
    def test_block_ends_at_sibling_key(self):
        self.assertEqual(check.exclusion_lines(GOLANGCI), {4, 5, 6})


if __name__ == "__main__":
    unittest.main()
