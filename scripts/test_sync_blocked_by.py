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
        self.pages = {}
        self.taken = set()
        self.calls = []

    def issue_number(self, path: str) -> str:
        return path.split("/issues/")[1].split("/")[0]

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
        elif args[0] == "api" and "-X" not in args and any("dependencies/blocked_by" in a for a in args):
            path = next(a for a in args if "dependencies/blocked_by" in a)
            number = self.issue_number(path)
            if "--paginate" not in args or "per_page=100" not in path:
                err = "blocked_by read must paginate with per_page=100"
            else:
                pages = self.pages.get(number)
                if pages is None:
                    out = json.dumps(self.links.get(number, []))
                else:
                    out = "\n".join(json.dumps(page) for page in pages)
        elif args[:2] == ["api", "-X"] and args[2] == "POST" and self.issue_number(args[3]) in self.taken:
            err = "Validation failed: Target issue has already been taken (HTTP 422)"
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
                ["api", "-X", "DELETE", "repos/acme/app/issues/10/dependencies/blocked_by/8000"],
                ["api", "-X", "POST", "repos/acme/app/issues/10/dependencies/blocked_by", "-F", "issue_id=5000"],
                ["api", "-X", "DELETE", "repos/acme/app/issues/11/dependencies/blocked_by/9000"],
            ],
        )
        self.assertNotIn("would", out)

    def test_repo_flag_skips_the_repo_lookup(self):
        code, _, err = self.run_sync("--repo", "other/repo")
        self.assertEqual(code, 0, err)
        self.assertNotIn(["repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner"], self.gh.calls)
        self.assertIn(
            ["api", "--paginate", "repos/other/repo/issues/10/dependencies/blocked_by?per_page=100"],
            self.gh.calls,
        )

    def test_a_gh_failure_exits_nonzero_and_names_the_call(self):
        self.gh.broken = 5
        code, _, err = self.run_sync("--apply")
        self.assertEqual(code, 1)
        self.assertIn("repos/acme/app/issues/5", err)
        self.assertIn("HTTP 500", err)
        self.assertEqual(
            self.gh.writes(),
            [["api", "-X", "DELETE", "repos/acme/app/issues/10/dependencies/blocked_by/8000"]],
        )

    def test_milestone_and_tracking_are_required(self):
        stderr = io.StringIO()
        with contextlib.redirect_stderr(stderr), self.assertRaises(SystemExit) as exit:
            sync.main([])
        self.assertEqual(exit.exception.code, 2)
        self.assertIn("--milestone", stderr.getvalue())

    def test_thirty_five_links_across_two_pages_are_in_sync(self):
        numbers = list(range(1, 36))
        self.gh.issues.append({"number": 14, "body": header(", ".join(f"#{n}" for n in numbers))})
        self.gh.pages["14"] = [
            [{"number": n, "id": n * 1000} for n in numbers[:30]],
            [{"number": n, "id": n * 1000} for n in numbers[30:]],
        ]
        code, out, err = self.run_sync()
        self.assertEqual(code, 0, err)
        self.assertIn(f"#14: in sync {numbers}", out)
        self.assertEqual(self.gh.writes(), [])

    def test_a_removal_past_the_first_page_is_applied(self):
        kept = list(range(1, 31))
        self.gh.issues.append({"number": 15, "body": header(", ".join(f"#{n}" for n in kept))})
        self.gh.pages["15"] = [
            [{"number": n, "id": n * 1000} for n in kept],
            [{"number": 99, "id": 99000}],
        ]
        code, _, err = self.run_sync("--apply")
        self.assertEqual(code, 0, err)
        deletes = [c for c in self.gh.writes() if c[2] == "DELETE" and "/issues/15/" in c[3]]
        self.assertEqual(
            deletes,
            [["api", "-X", "DELETE", "repos/acme/app/issues/15/dependencies/blocked_by/99000"]],
        )

    def test_already_taken_on_one_add_does_not_stop_the_run(self):
        self.gh.issues.append({"number": 16, "body": header("#5")})
        self.gh.links["16"] = []
        self.gh.taken.add("16")
        self.gh.issues.append({"number": 17, "body": header("none")})
        self.gh.links["17"] = [{"number": 9, "id": 17000}]
        code, out, err = self.run_sync("--apply")
        self.assertEqual(code, 0, err)
        self.assertIn("#16: #5 already in sync:", out)
        self.assertIn("already been taken", out)
        self.assertIn("HTTP 422", out)
        self.assertIn(
            ["api", "-X", "POST", "repos/acme/app/issues/16/dependencies/blocked_by", "-F", "issue_id=5000"],
            self.gh.writes(),
        )
        self.assertIn(
            ["api", "-X", "DELETE", "repos/acme/app/issues/17/dependencies/blocked_by/17000"],
            self.gh.writes(),
        )


if __name__ == "__main__":
    unittest.main()
