import importlib.util
import pathlib
import unittest

spec = importlib.util.spec_from_file_location(
    "check_pr_format", pathlib.Path(__file__).with_name("check-pr-format.py")
)
check = importlib.util.module_from_spec(spec)
spec.loader.exec_module(check)

GOOD_BODY = "\n\n".join(f"## {s}\n\nText for {s}." for s in check.SECTIONS)


def issues(kinds):
    return lambda repo, number: kinds.get(number, "missing")


class TitleTest(unittest.TestCase):
    def test_accepts_issue_number_and_descriptor(self):
        self.assertEqual(check.title_errors("#212 Add errs code table", "o/r", issues({"212": "issue"})), [])

    def test_rejects_conventional_commit_title(self):
        self.assertEqual(len(check.title_errors("docs: add stuff", "o/r", issues({}))), 1)

    def test_rejects_number_without_space_or_descriptor(self):
        self.assertEqual(len(check.title_errors("#212", "o/r", issues({"212": "issue"}))), 1)
        self.assertEqual(len(check.title_errors("#212Add", "o/r", issues({"212": "issue"}))), 1)

    def test_rejects_missing_issue(self):
        errors = check.title_errors("#999 Something", "o/r", issues({}))
        self.assertIn("does not exist", errors[0])

    def test_rejects_pull_request_number(self):
        errors = check.title_errors("#451 Something", "o/r", issues({"451": "pull_request"}))
        self.assertIn("pull request", errors[0])


class BodyTest(unittest.TestCase):
    def test_accepts_all_six_sections_with_text(self):
        self.assertEqual(check.body_errors(GOOD_BODY), [])

    def test_accepts_none_as_content(self):
        body = GOOD_BODY.replace("Text for Reviewer focus.", "None.")
        self.assertEqual(check.body_errors(body), [])

    def test_rejects_missing_section(self):
        body = GOOD_BODY.replace("## What came up", "## Notes")
        errors = check.body_errors(body)
        self.assertTrue(any('"## What came up"' in e for e in errors))

    def test_rejects_section_holding_only_template_comment(self):
        body = GOOD_BODY.replace("Text for Proof.", "<!-- Exact commands and what they printed. -->")
        errors = check.body_errors(body)
        self.assertEqual(errors, ['"## Proof" is empty; write the content or "None."'])

    def test_rejects_unfilled_template(self):
        template = pathlib.Path(__file__).parents[1].joinpath(".github/pull_request_template.md").read_text()
        self.assertEqual(len(check.body_errors(template)), 6)

    def test_rejects_empty_body(self):
        self.assertEqual(len(check.body_errors("")), 6)


if __name__ == "__main__":
    unittest.main()
