import contextlib
import importlib.util
import io
import os
import pathlib
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


class FakeRepo:
    def __init__(self, lines=None, unverified=(), off=()):
        self.heads = {11: "h11", 12: "h12", 13: "h13"}
        self.sizes = lines or {("base", "h11"): 400, ("h11", "h12"): 900, ("h12", "h13"): 300}
        self.unverified, self.off = set(unverified), set(off)

    def head(self, number):
        return self.heads[number]

    def verified(self, sha):
        return sha not in self.unverified

    def lines(self, parent, sha):
        return self.sizes[(parent, sha)]

    def on_top_of(self, parent, sha):
        return (parent, sha) not in self.off


class StackTest(unittest.TestCase):
    def test_reads_only_a_first_line_lands_stack(self):
        self.assertEqual(check.stack_numbers("Lands stack: #11 #12 #13\n\n## TLDR"), [11, 12, 13])
        self.assertEqual(check.stack_numbers("Lands stack:"), [])
        self.assertIsNone(check.stack_numbers("## TLDR\nLands stack: #11"))
        self.assertIsNone(check.stack_numbers(""))
        self.assertIsNone(check.stack_numbers(None))

    def test_a_verified_stack_of_small_prs_passes(self):
        self.assertEqual(check.stack_errors([11, 12, 13], 13, "base", FakeRepo()), [])

    def test_each_failure_is_named(self):
        repo = FakeRepo(
            lines={("base", "h11"): 400, ("h11", "h12"): 1000, ("h12", "h13"): 300},
            unverified={"h11"},
            off={("h12", "h13")},
        )
        self.assertEqual(check.stack_errors([11, 12, 13], 13, "base", repo), [
            "#11 has no verify success on its head",
            "#12 is 1000 lines against its parent",
            "#13 does not sit on #12",
        ])

    def test_the_line_must_end_with_this_pr(self):
        for numbers in ([], [11, 12], [13, 12]):
            errors = check.stack_errors(numbers, 13, "base", FakeRepo())
            self.assertEqual(len(errors), 1)
            self.assertIn("end with this PR, #13", errors[0])


class RepoTest(unittest.TestCase):
    def verified(self, statuses):
        done = mock.Mock(stdout=statuses)
        with mock.patch.object(check.subprocess, "run", return_value=done) as run:
            got = check.Repo("o/r").verified("h1")
        self.assertEqual(run.call_args[0][0], ["gh", "api", "repos/o/r/commits/h1/statuses?per_page=100"])
        return got

    def test_the_latest_verify_status_decides(self):
        self.assertTrue(self.verified('[{"context":"verify","state":"success"},{"context":"verify","state":"failure"}]'))
        self.assertFalse(self.verified('[{"context":"verify","state":"failure"},{"context":"verify","state":"success"}]'))
        self.assertFalse(self.verified('[{"context":"ci","state":"success"}]'))


class MainTest(unittest.TestCase):
    ENV = {"BASE_SHA": "base", "HEAD_SHA": "h13", "PR_NUMBER": "13", "GITHUB_REPOSITORY": "o/r", "PR_LABELS": "[]"}

    def run_main(self, body, repo, labels="[]", size=2000):
        env = dict(self.ENV, PR_BODY=body, PR_LABELS=labels)
        out = io.StringIO()
        with mock.patch.dict(os.environ, env, clear=True), \
                mock.patch.object(check, "numstat", return_value=f"{size}\t0\tapps/backend/x.go\n"), \
                mock.patch.object(check, "Repo", return_value=repo), contextlib.redirect_stdout(out):
            return check.main(), out.getvalue()

    def test_under_the_limit_passes(self):
        self.assertEqual(self.run_main("## TLDR", FakeRepo(), size=999)[0], 0)

    def test_over_the_limit_without_a_stack_fails(self):
        code, out = self.run_main("## TLDR", FakeRepo())
        self.assertEqual(code, 1)
        self.assertIn("Split it into a Graphite stack", out)
        self.assertIn("large-pr", out)

    def test_the_label_passes(self):
        self.assertEqual(self.run_main("## TLDR", FakeRepo(), labels='["large-pr"]')[0], 0)

    def test_a_verified_stack_passes(self):
        code, out = self.run_main("Lands stack: #11 #12 #13\n\n## TLDR", FakeRepo())
        self.assertEqual(code, 0)
        self.assertIn("lands a verified stack", out)

    def test_a_stack_with_an_unverified_pr_fails_and_says_why(self):
        code, out = self.run_main("Lands stack: #11 #12 #13\n\n## TLDR", FakeRepo(unverified={"h12"}))
        self.assertEqual(code, 1)
        self.assertIn("#12 has no verify success on its head", out)
        self.assertIn("large-pr", out)


if __name__ == "__main__":
    unittest.main()
