import contextlib
import importlib.util
import io
import json
import pathlib
import subprocess
import unittest
from unittest import mock

spec = importlib.util.spec_from_file_location(
    "sync_blocked_by", pathlib.Path(__file__).with_name("sync-blocked-by.py")
)
sync = importlib.util.module_from_spec(spec)
spec.loader.exec_module(sync)


def header(blockers: str) -> str:
    return f"**Milestone:** M1 · **Blocked by:** {blockers} · **Tracking:** #1 · **Touches:** `x`\n\n## Context\nsee #99\n"


class FakeGh:
    def __init__(self):
        self.issues = [
            {"number": 1, "body": "tracking table, blocked by #5"},
            {"number": 10, "body": header("#5, #6")},
            {"number": 11, "body": header("none")},
            {"number": 12, "body": "no header here\n**Blocked by:** #5"},
            {"number": 13, "body": header("#7")},
        ]
        self.links = {
            "10": [{"number": 6, "id": 6000}, {"number": 8, "id": 8000}],
            "11": [{"number": 9, "id": 9000}],
            "13": [{"number": 7, "id": 7000}],
        }
        self.broken = None
        self.calls = []

    def run(self, argv, **_):
        assert argv[0] == "gh", argv
        args = argv[1:]
        self.calls.append(args)
        out, err = "", ""
        if args[:2] == ["repo", "view"]:
            out = "acme/app\n"
        elif args[:2] == ["issue", "list"]:
            out = json.dumps(self.issues) if args[args.index("--milestone") + 1] == "M1" else ""
            err = "" if out else "unknown milestone"
        elif args[0] == "api" and args[1].endswith("/dependencies/blocked_by"):
            out = json.dumps(self.links.get(args[1].split("/")[4], []))
        elif args[:2] == ["api", "-X"]:
            out = "{}"
        elif args[0] == "api":
            n = int(args[1].rsplit("/", 1)[1])
            out, err = ("", "HTTP 500") if n == self.broken else (json.dumps({"id": n * 1000}), "")
        else:
            err = "unexpected gh call"
        return subprocess.CompletedProcess(argv, 1 if err else 0, out, err)

    def writes(self):
        return [c for c in self.calls if c[:2] == ["api", "-X"]]


class SyncBlockedByTest(unittest.TestCase):
    def setUp(self):
        self.gh = FakeGh()
        patcher = mock.patch.object(sync.subprocess, "run", self.gh.run)
        patcher.start()
        self.addCleanup(patcher.stop)

    def run_sync(self, *args):
        stdout, stderr = io.StringIO(), io.StringIO()
        with contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(stderr):
            try:
                code = sync.main(["--milestone", "M1", "--tracking", "1", *args])
            except SystemExit as exit:
                code = exit.code
        return code, stdout.getvalue(), stderr.getvalue()

    def test_dry_run_prints_the_plan_and_changes_nothing(self):
        code, out, err = self.run_sync()
        self.assertEqual(code, 0, err)
        self.assertIn("#10: header [5, 6] links [6, 8]; would add [5] would remove [8]", out)
        self.assertIn("#11: header [] links [9]; would add [] would remove [9]", out)
        self.assertIn("#12: no Blocked by header; links []", out)
        self.assertIn("#13: in sync [7]", out)
        self.assertNotIn("#1:", out)
        self.assertIn("rerun with --apply", out)
        self.assertEqual(self.gh.writes(), [])

    def test_apply_adds_missing_links_and_removes_stale_ones(self):
        code, out, err = self.run_sync("--apply")
        self.assertEqual(code, 0, err)
        self.assertEqual(
            self.gh.writes(),
            [
                ["api", "-X", "POST", "repos/acme/app/issues/10/dependencies/blocked_by", "-F", "issue_id=5000"],
                ["api", "-X", "DELETE", "repos/acme/app/issues/10/dependencies/blocked_by/8000"],
                ["api", "-X", "DELETE", "repos/acme/app/issues/11/dependencies/blocked_by/9000"],
            ],
        )
        self.assertNotIn("would", out)

    def test_repo_flag_skips_the_repo_lookup(self):
        code, _, err = self.run_sync("--repo", "other/repo")
        self.assertEqual(code, 0, err)
        self.assertNotIn(["repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner"], self.gh.calls)
        self.assertIn(["api", "repos/other/repo/issues/10/dependencies/blocked_by"], self.gh.calls)

    def test_a_gh_failure_exits_nonzero_and_names_the_call(self):
        self.gh.broken = 5
        code, _, err = self.run_sync("--apply")
        self.assertEqual(code, 1)
        self.assertIn("repos/acme/app/issues/5", err)
        self.assertIn("HTTP 500", err)
        self.assertEqual(self.gh.writes(), [])

    def test_milestone_and_tracking_are_required(self):
        stderr = io.StringIO()
        with contextlib.redirect_stderr(stderr), self.assertRaises(SystemExit) as exit:
            sync.main([])
        self.assertEqual(exit.exception.code, 2)
        self.assertIn("--milestone", stderr.getvalue())


if __name__ == "__main__":
    unittest.main()
