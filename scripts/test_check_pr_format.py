from __future__ import annotations

import importlib.util
import os
import pathlib
import subprocess
import tempfile
import unittest
from contextlib import redirect_stdout
from io import StringIO
from unittest import mock

spec = importlib.util.spec_from_file_location(
    "check_pr_format", pathlib.Path(__file__).with_name("check-pr-format.py")
)
check = importlib.util.module_from_spec(spec)
spec.loader.exec_module(check)

GOOD_BODY = "\n\n".join(f"## {s}\n\nText for {s}." for s in check.SECTIONS)


def pr(why: str, needs: str | None = None) -> str:
    body = GOOD_BODY.replace("Text for Why.", why)
    return body if needs is None else f"{body}\n\n## Needs from Logan\n\n{needs}\n"


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



class TicketTest(unittest.TestCase):
    def test_accepts_part_of_with_no_neighbours(self):
        self.assertEqual(check.ticket_errors(pr("Part of #789."), [], []), [])

    def test_accepts_closes_on_the_last_pr_with_needs(self):
        below = [(10, pr("Part of #789."))]
        self.assertEqual(check.ticket_errors(pr("Closes #789.", "Nothing."), [], below), [])

    def test_rejects_a_body_with_no_ticket_link(self):
        self.assertIn("links no ticket", check.ticket_errors(pr("It was slow."), [], [])[0])

    def test_reads_the_link_from_why_only(self):
        body = pr("Part of #789.").replace("Text for Proof.", "A planted Closes #789 on a lower PR failed.")
        self.assertEqual(check.ticket_errors(body, [(11, pr("Part of #789."))], []), [])

    def test_rejects_closes_on_a_pr_with_the_same_ticket_above_it(self):
        errors = check.ticket_errors(pr("Closes #789.", "Nothing."), [(11, pr("Part of #789."))], [])
        self.assertEqual(len(errors), 1)
        self.assertIn("#11 above it is part of #789", errors[0])

    def test_rejects_every_github_closing_keyword_on_a_non_last_pr(self):
        for verb in ["Fixes", "resolves", "closed", "Fix"]:
            errors = check.ticket_errors(pr(f"{verb} #789.", "Nothing."), [(11, pr("Part of #789."))], [])
            self.assertEqual(len(errors), 1, verb)

    def test_rejects_part_of_above_a_pr_that_closes_the_ticket(self):
        errors = check.ticket_errors(pr("Part of #789."), [], [(10, pr("Closes #789.", "Nothing."))])
        self.assertEqual(len(errors), 1)
        self.assertIn('#10 below this PR closes #789', errors[0])

    def test_ignores_neighbours_of_another_ticket(self):
        body = pr("Closes #789.", "Nothing.")
        self.assertEqual(check.ticket_errors(body, [(11, pr("Part of #790."))], [(10, pr("Closes #788.", "x"))]), [])

    def test_rejects_closes_without_needs_from_logan(self):
        errors = check.ticket_errors(pr("Closes #789."), [], [])
        self.assertEqual(len(errors), 1)
        self.assertIn('needs a "## Needs from Logan" section', errors[0])

    def test_rejects_an_empty_needs_from_logan(self):
        errors = check.ticket_errors(pr("Closes #789.", "<!-- fill me -->"), [], [])
        self.assertEqual(errors, ['"## Needs from Logan" is empty; write "Nothing." or the checklist'])


class CommandTest(unittest.TestCase):
    def test_accepts_commands_that_parse(self):
        needs = "- [ ] Add the label:\n\n```bash\ngh issue edit 789 --add-label x\n```\n\n```\nfor i in 1 2; do echo $i; done\n```"
        self.assertEqual(check.command_errors(pr("Closes #789.", needs)), [])

    def test_rejects_a_command_that_does_not_parse(self):
        needs = "```sh\ngh api repos/x --jq '.[] | {name\n```"
        errors = check.command_errors(pr("Closes #789.", needs))
        self.assertEqual(len(errors), 1)
        self.assertIn("does not parse", errors[0])

    def test_skips_non_shell_fences_and_other_sections(self):
        needs = "```json\n{\"a\": (\n```"
        body = pr("Closes #789.", needs).replace("Text for Proof.", "```\nif then\n```")
        self.assertEqual(check.command_errors(body), [])


def git(*args: str) -> str:
    return subprocess.run(["git", *args], capture_output=True, text=True, check=True).stdout.strip()


def commit(subject: str) -> str:
    git("commit", "-q", "--allow-empty", "-m", subject)
    return git("rev-parse", "HEAD")


class RepoTest(unittest.TestCase):
    def setUp(self):
        self.env = dict(os.environ)
        os.environ.update(
            GIT_AUTHOR_DATE="2026-01-01T00:00:00Z",
            GIT_COMMITTER_DATE="2026-01-01T00:00:00Z",
            GIT_CONFIG_GLOBAL=os.devnull,
            GIT_CONFIG_NOSYSTEM="1",
        )
        self.cwd = os.getcwd()
        self.dir = tempfile.TemporaryDirectory()
        os.chdir(self.dir.name)
        git("init", "-q", "-b", "trunk")
        git("config", "user.email", "t@example.com")
        git("config", "user.name", "t")
        self.base = commit("Add the base without a type prefix")

    def tearDown(self):
        os.chdir(self.cwd)
        os.environ.clear()
        os.environ.update(self.env)
        self.dir.cleanup()


class ShaTest(RepoTest):
    def test_accepts_ancestors_short_and_full(self):
        head = commit("feat: head")
        self.assertRegex(self.base[:8], check.SHA_RE)
        body = pr(f"Part of #789. Parent trunk@{self.base[:8]}.").replace("Text for Proof.", f"Head {head}.")
        self.assertEqual(check.sha_errors(body, head), [])

    def test_rejects_a_sha_that_is_not_an_ancestor(self):
        git("switch", "-q", "-c", "side")
        side = commit("feat: side")
        git("switch", "-q", "trunk")
        head = commit("feat: head")
        self.assertRegex(side[:10], check.SHA_RE)
        errors = check.sha_errors(pr(f"Part of #789. Built at {side[:10]}."), head)
        self.assertEqual(len(errors), 1)
        self.assertIn("not an ancestor of the head", errors[0])

    def test_rejects_a_sha_missing_from_the_repository(self):
        errors = check.sha_errors(pr("Part of #789. Built at 0123abc4."), self.base)
        self.assertEqual(errors, ["the body cites 0123abc4, which is not a commit in this repository"])

    def test_ignores_hex_that_is_not_a_sha(self):
        body = pr("Part of #789. Run 12345678, session d7588746-98d4-460f-bd63-d2af37e65b75, word defaced.")
        self.assertEqual(check.sha_errors(body, self.base), [])


class CommitTest(RepoTest):
    def test_accepts_conventional_subjects(self):
        for subject in ["feat: add ready job", "fix(ci)!: fail on a stale file", "revert: undo x", "ci(pr-format): lint"]:
            commit(subject)
        self.assertEqual(check.commit_errors(self.base, git("rev-parse", "HEAD")), [])

    def test_rejects_a_subject_without_a_type(self):
        sha = commit("Add the ready job")
        commit("feat: fine")
        errors = check.commit_errors(self.base, git("rev-parse", "HEAD"))
        self.assertEqual(len(errors), 1)
        self.assertIn(f'commit {sha[:12]} "Add the ready job" is not a Conventional Commit', errors[0])
        self.assertIn("/commit skill", errors[0])

    def test_rejects_an_unknown_type_and_a_missing_space(self):
        commit("feature: x")
        commit("feat:x")
        self.assertEqual(len(check.commit_errors(self.base, git("rev-parse", "HEAD"))), 2)

    def test_skips_merge_commits_and_squash_merges(self):
        git("switch", "-q", "-c", "side")
        commit("feat: side")
        git("switch", "-q", "trunk")
        commit("Rewrite the backend on the M7 platform (#786)")
        git("merge", "-q", "--no-ff", "-m", "Merge branch 'side' into trunk", "side")
        self.assertEqual(check.commit_errors(self.base, git("rev-parse", "HEAD")), [])


class CheckpointTest(RepoTest):
    def run_check(self, base_ref: str, head_ref: str, labels: str = '["integration"]') -> tuple[int, str]:
        commit("Log bus.relay.idle at most once a minute")
        os.environ.update(
            PR_TITLE="Merge the backend rewrite",
            PR_BODY=pr("Part of #789."),
            BASE_REF=base_ref,
            HEAD_REF=head_ref,
            PR_LABELS=labels,
            BASE_SHA=self.base,
            HEAD_SHA=git("rev-parse", "HEAD"),
        )
        out = StringIO()
        with mock.patch.object(check, "stacked", return_value=[]), redirect_stdout(out):
            code = check.main([])
        return code, out.getvalue()

    def test_checkpoint_into_main_skips_the_commit_check(self):
        for head in ("backend-rewrite-9", "domain-core-12"):
            self.assertEqual(self.run_check("main", head), (0, "PR format ok\n"), head)

    def test_feature_branch_into_main_without_the_integration_label_still_fails(self):
        for labels in ("[]", '["docs"]', ""):
            self.assertEqual(self.run_check("main", "domain-core-12", labels)[0], 1, labels)

    def test_is_checkpoint_needs_main_the_convention_and_the_label(self):
        for head in ("backend-rewrite-3", "domain-core-12"):
            self.assertTrue(check.is_checkpoint("main", head, ["integration"]), head)
        for head in ("backend-rewrite", "982-workflow-docs", "main", "Domain-Core-2"):
            self.assertFalse(check.is_checkpoint("main", head, ["integration"]), head)
        self.assertFalse(check.is_checkpoint("domain-core-11", "domain-core-12", ["integration"]))

    def test_ticket_pr_still_fails_on_a_non_conventional_commit(self):
        code, out = self.run_check("backend-rewrite-9", "989-checkpoint-commit-check")
        self.assertEqual(code, 1)
        self.assertIn('"Log bus.relay.idle at most once a minute" is not a Conventional Commit', out)

    def test_other_branch_into_main_still_fails(self):
        self.assertEqual(self.run_check("main", "hotfix")[0], 1)


if __name__ == "__main__":
    unittest.main()
