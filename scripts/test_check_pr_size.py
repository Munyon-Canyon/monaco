import contextlib
import importlib.util
import io
import os
import pathlib
import tempfile
import unittest
from unittest import mock

spec = importlib.util.spec_from_file_location(
    "check_pr_size", pathlib.Path(__file__).with_name("check-pr-size.py")
)
check = importlib.util.module_from_spec(spec)
spec.loader.exec_module(check)


class CountTest(unittest.TestCase):
    def test_counts_added_and_deleted_lines(self):
        total, _ = check.count("10\t5\tapps/backend/internal/app/fund.go\n3\t0\tREADME.md\n")
        self.assertEqual(total, 18)

    def test_skips_generated_and_lock_files(self):
        numstat = (
            "400\t0\tapps/backend/internal/db/queries.gen.go\n"
            "900\t12\tapps/backend/go.sum\n"
            "1878\t1852\tapps/backend/api/openapi.yaml\n"
            "50\t0\tapps/backend/test/evidence/07.json\n"
            "7\t1\tapps/backend/internal/app/fund.go\n"
        )
        self.assertEqual(check.count(numstat)[0], 8)

    def test_skips_binary_files(self):
        self.assertEqual(check.count("-\t-\tdocs/legacy/qa/home.png\n")[0], 0)

    def test_pure_rename_counts_zero(self):
        self.assertEqual(check.count("0\t0\tdocs/{archive => legacy}/README.md\n")[0], 0)

    def test_rename_with_edits_counts_edits_under_new_path(self):
        total, counted = check.count("4\t2\tdocs/{archive => legacy}/README.md\n")
        self.assertEqual(total, 6)
        self.assertEqual(counted[0][1], "docs/legacy/README.md")

    def test_renamed_path_forms(self):
        self.assertEqual(check.renamed_path("a/{b => c}/d.go"), "a/c/d.go")
        self.assertEqual(check.renamed_path("old.go => new.go"), "new.go")
        self.assertEqual(check.renamed_path("plain.go"), "plain.go")

    def test_limit_is_1000(self):
        self.assertEqual(check.LIMIT, 1000)


class BinaryTest(unittest.TestCase):
    def test_names_each_binary_that_is_not_media_or_testdata(self):
        numstat = (
            "-\t-\tapps/backend/api\n"
            "-\t-\tapps/backend/internal/x/testdata/blob.bin\n"
            "-\t-\ttestdata/root.bin\n"
            "-\t-\tdocs/legacy/qa/home.png\n"
            "-\t-\tapps/web/assets/x.png\n"
            "-\t-\t.qa-screenshots/home.JPEG\n"
            "-\t-\tapps/mobile/Monaco/Assets/icon.jpg\n"
            "-\t-\tdocs/rfc.pdf\n"
            "-\t-\tapps/web/fonts/inter.woff2\n"
            "-\t-\tapps/backend/internal/db/schema.gen\n"
            "-\t-\tapps/backend/lib/libfoo.so\n"
            "-\t-\t{bin => apps/backend/cmd}/worker\n"
            "7\t1\tapps/backend/internal/app/fund.go\n"
        )
        self.assertEqual(check.binaries(numstat), [
            "apps/backend/api",
            "apps/backend/internal/db/schema.gen",
            "apps/backend/lib/libfoo.so",
            "apps/backend/cmd/worker",
        ])


class MainTest(unittest.TestCase):
    ENV = {"BASE_SHA": "base", "HEAD_SHA": "h13", "PR_LABELS": "[]"}

    def run_main(self, labels="[]", size=2000, extra=""):
        env = dict(self.ENV, PR_LABELS=labels)
        out = io.StringIO()
        with mock.patch.dict(os.environ, env, clear=True), \
                mock.patch.object(check, "numstat", return_value=f"{size}\t0\tapps/backend/x.go\n{extra}"), \
                contextlib.redirect_stdout(out):
            return check.main(), out.getvalue()

    def test_a_binary_under_apps_backend_fails_whatever_the_label(self):
        code, out = self.run_main(labels='["large-pr"]', size=10,
                                  extra="-\t-\tapps/backend/worker\n")
        self.assertEqual(code, 1)
        self.assertIn("  apps/backend/worker\n", out)

    def test_a_png_under_apps_web_passes(self):
        code, _ = self.run_main(size=10, extra="-\t-\tapps/web/assets/x.png\n")
        self.assertEqual(code, 0)

    def test_a_binary_under_testdata_passes(self):
        code, _ = self.run_main(size=10,
                                extra="-\t-\tapps/backend/internal/x/testdata/blob.bin\n")
        self.assertEqual(code, 0)

    def test_under_the_limit_passes(self):
        self.assertEqual(self.run_main(size=999)[0], 0)

    def test_over_the_limit_fails(self):
        code, out = self.run_main()
        self.assertEqual(code, 1)
        self.assertIn("Split it into a Graphite stack", out)
        self.assertIn("large-pr", out)

    def fast_track(self, size, path, **env):
        diff = f"{size}\t0\t{path}\n"
        out = io.StringIO()
        with mock.patch.dict(os.environ, dict(self.ENV, PR_LABELS='["fast-track"]', **env), clear=True), \
                mock.patch.object(check, "numstat", return_value=diff), contextlib.redirect_stdout(out):
            return check.main(), out.getvalue()

    def test_fast_track_under_100_lines_outside_the_backend_passes(self):
        for path in ("apps/mobile/Monaco/X.swift", "docs/how-to/x.md", "scripts/x.sh"):
            self.assertEqual(self.fast_track(99, path)[0], 0, path)

    def test_fast_track_on_a_heavy_path_fails_whatever_the_size(self):
        for path in ("apps/backend/internal/x.go", ".github/workflows/ci.yml", "docker-compose.yml",
                     "apps/backend/go.sum", "apps/backend/.golangci.yml", "apps/backend/internal/x/x.gen.go"):
            code, out = self.fast_track(1, path)
            self.assertEqual(code, 1, path)
            self.assertIn(f"it touches {path}", out)

    def test_fast_track_counts_both_sides_of_a_rename_and_binaries(self):
        for line in ("0\t0\tapps/backend/x.md => docs/x.md", "0\t0\t{apps/backend => docs}/x.md",
                     "-\t-\tapps/backend/internal/x/testdata/blob.bin"):
            out = io.StringIO()
            env = dict(self.ENV, PR_LABELS='["fast-track"]')
            with mock.patch.dict(os.environ, env, clear=True), \
                    mock.patch.object(check, "numstat", return_value=line + "\n"), contextlib.redirect_stdout(out):
                self.assertEqual(check.main(), 1, line)
            self.assertIn("it touches apps/backend/", out.getvalue())

    def test_fast_track_at_100_lines_fails_and_asks_to_drop_the_label(self):
        with tempfile.NamedTemporaryFile("r") as gh_out:
            code, out = self.fast_track(100, "docs/how-to/x.md", GITHUB_OUTPUT=gh_out.name)
            self.assertEqual(code, 1)
            self.assertIn("for PRs under 100 lines outside apps/backend/, .github/, docker-compose.yml; it has 100 lines", out)
            self.assertEqual(gh_out.read(), "fast_track_too_big=true\n")

    def test_the_label_passes(self):
        self.assertEqual(self.run_main(labels='["large-pr"]')[0], 0)


if __name__ == "__main__":
    unittest.main()
