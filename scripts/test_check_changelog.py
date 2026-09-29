import contextlib
import importlib.util
import io
import os
import pathlib
import subprocess
import tempfile
import unittest

spec = importlib.util.spec_from_file_location(
    "check_changelog", pathlib.Path(__file__).with_name("check-changelog.py")
)
check = importlib.util.module_from_spec(spec)
spec.loader.exec_module(check)

GO = "apps/backend/internal/system/ping.go"
LOG = "apps/backend/CHANGELOG.md"

EMPTY = """# Changelog

## [Unreleased]

### Added

- Nothing yet.

## [checkpoint 1] - 2026-09-27

### Added

- The api binary.
"""

UNRELEASED = EMPTY.replace("- Nothing yet.", "- `POST /v1/system/pings`.")

RENAMED = """# Changelog

## [Unreleased]

## [checkpoint 2] - 2026-10-01

### Added

- `POST /v1/system/pings`.

## [checkpoint 1] - 2026-09-27

### Added

- The api binary.
"""


class ProblemsTest(unittest.TestCase):
    def test_backend_change_without_the_changelog_fails_and_names_the_file(self):
        found = check.problems([GO], UNRELEASED, EMPTY)
        self.assertEqual(len(found), 1)
        self.assertIn(LOG, found[0])
        self.assertIn("### Added", found[0])
        self.assertIn("Keep a Changelog", found[0])

    def test_backend_change_with_unreleased_entries_passes(self):
        self.assertEqual(check.problems([GO, LOG], UNRELEASED, EMPTY), [])

    def test_empty_unreleased_fails_even_when_the_changelog_changed(self):
        found = check.problems([GO, LOG], EMPTY, EMPTY.replace("The api", "An api"))
        self.assertEqual(len(found), 1)
        self.assertIn("## [Unreleased]", found[0])
        self.assertIn("### Security", found[0])

    def test_checkpoint_rename_with_a_new_empty_unreleased_passes(self):
        self.assertEqual(check.problems([GO, LOG], RENAMED, UNRELEASED), [])

    def test_empty_unreleased_over_an_already_released_checkpoint_fails(self):
        self.assertEqual(len(check.problems([GO, LOG], RENAMED, RENAMED + "\n")), 1)

    def test_changes_outside_the_backend_pass(self):
        self.assertEqual(check.problems(["apps/mobile/App.swift", "docs/index.md"], None, None), [])

    def test_a_changelog_only_change_passes(self):
        self.assertEqual(check.problems([LOG], EMPTY, UNRELEASED), [])

    def test_a_deleted_changelog_fails(self):
        self.assertEqual(len(check.problems([GO, LOG], None, UNRELEASED)), 1)


class MainTest(unittest.TestCase):
    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        cwd = os.getcwd()
        self.addCleanup(os.chdir, cwd)
        os.chdir(tmp.name)
        self.root = pathlib.Path(tmp.name)
        self.git("init", "-q", "-b", "main")
        self.git("config", "user.email", "t@example.com")
        self.git("config", "user.name", "t")
        self.base = self.commit({LOG: EMPTY, GO: "package system\n"})

    def git(self, *args):
        return subprocess.run(["git", *args], cwd=self.root, check=True, capture_output=True, text=True).stdout

    def commit(self, files):
        for path, text in files.items():
            (self.root / path).parent.mkdir(parents=True, exist_ok=True)
            (self.root / path).write_text(text)
        self.git("add", "-A")
        self.git("commit", "-q", "-m", "change")
        return self.git("rev-parse", "HEAD").strip()

    def run_main(self, head):
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            code = check.main(["check-changelog.py", self.base, head])
        return code, out.getvalue()

    def test_planted_backend_change_without_an_entry_fails(self):
        head = self.commit({GO: "package system\n\nfunc Ping() {}\n"})
        code, out = self.run_main(head)
        self.assertEqual(code, 1)
        self.assertIn(f"::error file={LOG}::", out)

    def test_backend_change_with_an_entry_passes(self):
        head = self.commit({GO: "package system\n\nfunc Ping() {}\n", LOG: UNRELEASED})
        self.assertEqual(self.run_main(head)[0], 0)

    def test_a_rename_compares_against_the_merge_base_changelog(self):
        self.base = self.commit({LOG: UNRELEASED})
        head = self.commit({GO: "package system\n\nfunc Ping() {}\n", LOG: RENAMED})
        self.assertEqual(self.run_main(head)[0], 0)

    def test_wrong_arguments_print_usage(self):
        err = io.StringIO()
        with contextlib.redirect_stderr(err):
            self.assertEqual(check.main(["check-changelog.py", "only-base"]), 2)
        self.assertIn("usage: scripts/check-changelog.py <base> <head>", err.getvalue())


if __name__ == "__main__":
    unittest.main()
