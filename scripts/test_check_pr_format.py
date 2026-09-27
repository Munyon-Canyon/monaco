import importlib.util
import pathlib
import unittest

spec = importlib.util.spec_from_file_location(
    "check_pr_format", pathlib.Path(__file__).with_name("check-pr-format.py")
)
check = importlib.util.module_from_spec(spec)
spec.loader.exec_module(check)

GOOD_BODY = "\n\n".join(f"## {s}\n\nText for {s}." for s in check.SECTIONS)


class TitleTest(unittest.TestCase):
    def test_accepts_plain_descriptor(self):
        self.assertEqual(check.title_errors("Add errs code table and problem+json mapping"), [])

    def test_accepts_issue_reference_later_in_title(self):
        self.assertEqual(check.title_errors("Fix redeem rounding reported in #212"), [])

    def test_accepts_colon_that_is_not_a_commit_prefix(self):
        self.assertEqual(check.title_errors("Backend RFC: errors, logs and tests"), [])

    def test_rejects_leading_issue_number(self):
        self.assertIn("issue number", check.title_errors("#212 Add errs code table")[0])

    def test_rejects_commit_type_prefix(self):
        self.assertIn("commit-type prefix", check.title_errors("docs: add stuff")[0])
        self.assertIn("commit-type prefix", check.title_errors("feat(treasury)!: fund cabal")[0])

    def test_rejects_empty_title(self):
        self.assertEqual(check.title_errors("   "), ["title is empty"])


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
