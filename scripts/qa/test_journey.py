#!/usr/bin/env python3
"""Tests for journey.py: the doc parser, the doc-against-test checks, and the log readers.

Run: python3 scripts/qa/test_journey.py (all classes, in parallel shards), or name tests as unittest.main takes them.
"""

import inspect
import contextlib
import fcntl
import json
import multiprocessing
import os
import shutil
import signal
import socket
import subprocess
import sys
import tempfile
import threading
import time
import types
import unittest
import unittest.mock
from contextlib import redirect_stdout
from io import StringIO
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import journey  # noqa: E402

# No test takes a lock a real journey run on this Mac holds.
_LOCKS = tempfile.TemporaryDirectory()
journey.SLOT_LOCK = _LOCKS.name + "/slot%d.lock"
journey.ACTOR_LOCK = _LOCKS.name + "/actor-%s.lock"

# No test queues for a machine-wide simulator slot; the SimSlots tests use the real ones.
REAL_TAKE_SIM_SLOT, REAL_RELEASE_SIM_SLOT = journey.take_sim_slot, journey.release_sim_slot
journey.take_sim_slot = lambda udid: None
journey.release_sim_slot = lambda udid: None

DOC = """---
id: auth/sign-in
title: Sign in
version: 2          # bumped for the new button
milestone: M9
requires: []
actors: [A]
flows: [01]
xcuitest: [ui/SignInJourney.swift, ui/SignInJourneyUITests.swift]
---

# Sign in

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | The login form shows |

## Scenarios

### S1 Sign in

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S1.1 | tap | `a` | | b |
| S1.2 | type | `c` | x | y |

### S2 Relaunch

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S2.1 | relaunch | the app | | |
"""

JOURNEY_SWIFT = """enum SignInJourney {
    static let id = "auth/sign-in"
    static let version = 2
    static func run() { step("S1.1"); step("S1.2"); step("S2.1") }
}
"""

TESTS_SWIFT = """nonisolated final class SignInJourneyUITests: XCTestCase {
    func testJourney() throws {
        try session.scenario("S1") {}
        try session.scenario("S2") {}
    }
}
"""


class SharedBuildLock(unittest.TestCase):
    def test_the_shared_build_lock_is_not_the_name_xcode_lock_uses_for_its_folder(self):
        self.assertNotEqual(journey.shared_build_lock(), journey.OUT / (journey.SHARED_BUILD + ".lock"))
        self.assertEqual(journey.shared_build_lock().parent, journey.OUT)


class Tree(unittest.TestCase):
    """A throwaway repo with one journey, its test files and its accounts."""

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        root = Path(self.tmp.name)
        self.saved = (journey.ROOT, journey.DOCS, journey.QA)
        journey.ROOT, journey.DOCS, journey.QA = root, root / "docs" / "journeys", root / "qa"
        self.write("docs/journeys/auth/sign-in.md", DOC)
        self.write("docs/journeys/README.md", "# not a journey")
        self.write("ui/SignInJourney.swift", JOURNEY_SWIFT)
        self.write("ui/SignInJourneyUITests.swift", TESTS_SWIFT)
        self.write("qa/accounts.tsv", "# logins\nactor\tname\tphone\temail\tcode\nA\tAlfred\t555\ta@b.c\t123456\n")
        self.write("packages/flows/backend/01.tsv", "id\tflow\tstatus\n01\tSign in\tbuilt\n")

    def tearDown(self):
        journey.ROOT, journey.DOCS, journey.QA = self.saved
        self.tmp.cleanup()

    def write(self, relative, text):
        path = journey.ROOT / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text)
        return path

    def problems(self):
        return journey.check_journeys(journey.load_journeys(), journey.load_accounts(environ={}))


class FrontMatter(Tree):
    def test_reads_scalars_lists_nested_maps_and_drops_comments(self):
        loaded = journey.load_journeys()["auth/sign-in"]
        self.assertEqual(loaded.version, 2)
        self.assertEqual(loaded.requires, [])
        self.assertEqual(loaded.actors, ["A"])
        self.assertEqual(loaded.flows, ["01"])
        self.assertEqual(loaded.xcuitest, ["ui/SignInJourney.swift", "ui/SignInJourneyUITests.swift"])

    def test_reads_scenarios_and_steps_but_not_preconditions(self):
        loaded = journey.load_journeys()["auth/sign-in"]
        self.assertEqual(loaded.scenarios, ["S1", "S2"])
        self.assertEqual(loaded.steps, ["S1.1", "S1.2", "S2.1"])

    def test_a_doc_without_front_matter_is_refused(self):
        with self.assertRaises(journey.JourneyError):
            journey.parse_front_matter("# Sign in\n")


class Check(Tree):
    def test_a_doc_and_its_test_that_agree_pass(self):
        self.assertEqual(self.problems(), [])

    def test_a_test_built_from_an_older_version_is_named(self):
        self.write("ui/SignInJourney.swift", JOURNEY_SWIFT.replace("version = 2", "version = 1"))
        problems = self.problems()
        self.assertEqual(len(problems), 1)
        self.assertIn("ui/SignInJourney.swift: built from auth/sign-in version 1, the doc is at version 2", problems[0])

    def test_a_steps_file_without_the_stamp_is_named(self):
        self.write("ui/SignInJourney.swift", JOURNEY_SWIFT.replace('    static let id = "auth/sign-in"\n', ""))
        self.assertIn('ui/SignInJourney.swift: does not declare static let id = "auth/sign-in"', self.problems()[0])

    def test_a_missing_test_file_is_named(self):
        (journey.ROOT / "ui/SignInJourneyUITests.swift").unlink()
        self.assertIn("does not exist", self.problems()[0])

    def test_a_scenario_missing_from_test_journey_is_named(self):
        self.write("ui/SignInJourneyUITests.swift", TESTS_SWIFT.replace('        try session.scenario("S2") {}\n', ""))
        self.assertIn('testJourney has no session.scenario("S2")', self.problems()[0])

    def test_scenarios_out_of_the_docs_order_are_named(self):
        self.write("ui/SignInJourneyUITests.swift", TESTS_SWIFT.replace("S1", "SX").replace("S2", "S1").replace("SX", "S2"))
        self.assertIn("runs its scenarios out of the doc's order S1 S2", self.problems()[0])

    def test_per_scenario_test_methods_are_not_a_journey(self):
        self.write("ui/SignInJourneyUITests.swift", """nonisolated final class SignInJourneyUITests: XCTestCase {
    func testS1SignIn() throws {}
    func testS2Relaunch() throws {}
}
""")
        self.assertIn("no testJourney", self.problems()[0])

    def test_a_doc_step_the_test_does_not_record_is_named(self):
        self.write("ui/SignInJourney.swift", JOURNEY_SWIFT.replace('step("S1.2"); ', ""))
        self.assertTrue(any("no step S1.2" in p for p in self.problems()))

    def test_an_id_that_differs_from_the_path_is_named(self):
        self.write("docs/journeys/auth/sign-in.md", DOC.replace("id: auth/sign-in", "id: auth/login"))
        self.assertTrue(any("the path says 'auth/sign-in'" in p for p in self.problems()))

    def test_a_doc_without_an_id_is_named(self):
        self.write("docs/journeys/auth/sign-in.md", DOC.replace("id: auth/sign-in\n", ""))
        self.assertTrue(any("id is ''" in p for p in self.problems()))

    def test_duplicate_ids_are_named(self):
        self.write("docs/journeys/auth/again.md", DOC)
        self.assertTrue(any("duplicate id auth/sign-in in docs/journeys/auth/again.md and docs/journeys/auth/sign-in.md" in p
                            for p in self.problems()))

    def test_an_unknown_required_journey_and_actor_are_named(self):
        self.write("docs/journeys/auth/sign-in.md", DOC.replace("requires: []", "requires: [cabal/create]")
                   .replace("actors: [A]", "actors: [A, B]"))
        problems = "\n".join(self.problems())
        self.assertIn("requires 'cabal/create', which is not a journey doc", problems)
        self.assertIn("actor B has no row", problems)

    def test_a_seeded_bug_must_say_what_fails_and_must_apply(self):
        self.write("qa/auth/sign-in.mutants/broken.patch", "no expectation here\n")
        self.write("qa/auth/sign-in.mutants/stale.patch", "expect-fail: S1, S9\n")
        problems = "\n".join(journey.check_journeys(journey.load_journeys(), journey.load_accounts(environ={}), lambda patch: False))
        self.assertIn("broken.patch: no 'expect-fail: S…' line", problems)
        self.assertIn("stale.patch: expects S9 to fail", problems)
        self.assertIn("stale.patch: does not apply to this checkout for journey auth/sign-in", problems)
        self.assertIn("regenerate this patch with the ios-journey-qa skill", problems)

    def test_a_missing_backend_flow_is_named(self):
        self.write("docs/journeys/auth/sign-in.md", DOC.replace("flows: [01]", "flows: [99]"))
        self.assertIn("docs/journeys/auth/sign-in.md: flows names 99, which has no packages/flows/backend/99.tsv",
                      self.problems())


MIGRATIONS = """CREATE TABLE widgets (
  id uuid PRIMARY KEY,
  name text NOT NULL,
  note text,
  size int NOT NULL DEFAULT 1,
  PRIMARY KEY (id)
);
ALTER TABLE widgets ADD COLUMN colour text NOT NULL;
ALTER TABLE widgets
  ADD COLUMN weight int NOT NULL,
  ALTER COLUMN size DROP DEFAULT;
ALTER TABLE widgets ALTER COLUMN weight SET DEFAULT 0;
"""


GEN_DOC = DOC.replace("| S1.1 | tap | `a` | | b |", '| S1.1 | tap | `a` | | "Send code" shows within 5 s |').replace(
    "| S1.2 | type | `c` | x | y |", '| S1.2 | type, then tap | `c` | x | "QA vote {QA.run}" and "Done" |')


class Generated(Tree):
    PATH = "apps/mobile/MonacoUITests/Journeys/Generated/SignInJourneySteps.swift"

    def setUp(self):
        super().setUp()
        self.write("docs/journeys/auth/sign-in.md", GEN_DOC)

    def gen(self):
        with redirect_stdout(StringIO()):
            return journey.main(["gen", "auth/sign-in"])

    def test_gen_writes_one_value_per_step_row(self):
        self.assertEqual(self.gen(), 0)
        text = (journey.ROOT / self.PATH).read_text()
        self.assertIn("enum SignInJourneySteps {", text)
        self.assertIn('static let id = "auth/sign-in"', text)
        self.assertIn("static let version = 2\n", text)
        self.assertIn(
            'static let s1p1 = JourneyStep(id: "S1.1", name: "tap a", actor: "A", seconds: 5, copy: ["Send code"])', text)
        self.assertIn('        seconds: 15,\n        copy: ["QA vote {QA.run}", "Done"]\n', text)
        self.assertIn('static let s2p1 = JourneyStep(id: "S2.1", name: "relaunch the app", actor: "A", seconds: 15, copy: [])', text)
        self.assertNotIn("//", text)

    def test_gen_twice_changes_nothing(self):
        self.gen()
        first = (journey.ROOT / self.PATH).read_text()
        out = StringIO()
        with redirect_stdout(out):
            journey.main(["gen", "auth/sign-in"])
        self.assertEqual((journey.ROOT / self.PATH).read_text(), first)
        self.assertEqual(out.getvalue(), "")

    def test_a_generated_file_that_gen_would_write_differently_is_stale(self):
        self.assertEqual(self.problems(), [])
        self.gen()
        self.assertEqual(self.problems(), [])
        path = journey.ROOT / self.PATH
        path.write_text(path.read_text().replace("Send code", "Send a code"))
        problems = self.problems()
        self.assertEqual(len(problems), 1)
        self.assertIn("stale: run `scripts/qa/journey.py gen auth/sign-in`", problems[0])

    def test_the_stamp_is_read_from_the_generated_file(self):
        self.gen()
        self.write("ui/SignInJourney.swift", JOURNEY_SWIFT.replace('    static let id = "auth/sign-in"\n    static let version = 2\n', ""))
        self.assertEqual(self.problems(), [])


class VersionBump(Tree):
    def base(self, text):
        return lambda relative: text if relative == "docs/journeys/auth/sign-in.md" else None

    def problems_against(self, base):
        return journey.check_journeys(journey.load_journeys(), journey.load_accounts(environ={}), None, self.base(base))

    def test_a_step_change_without_a_bump_is_named(self):
        problems = self.problems_against(DOC.replace("| y |", "| z |"))
        self.assertEqual(len(problems), 1)
        self.assertIn("docs/journeys/auth/sign-in.md: the step tables changed against the merge base with origin/staging", problems[0])

    def test_a_step_change_with_a_bump_passes(self):
        self.assertEqual(self.problems_against(DOC.replace("| y |", "| z |").replace("version: 2", "version: 1")), [])

    def test_prose_changes_need_no_bump(self):
        self.assertEqual(self.problems_against(DOC.replace("# Sign in\n", "# Sign in\n\nWords.\n", 1)), [])

    def test_a_missing_base_is_skipped(self):
        self.assertEqual(self.problems_against(None), [])


class MergeBaseText(unittest.TestCase):
    def git(self, *args):
        subprocess.run(["git", "-c", "user.name=t", "-c", "user.email=t@t", *args], cwd=self.root, check=True, capture_output=True)

    def commit(self, text, message):
        (self.root / "doc.md").write_text(text)
        self.git("add", "doc.md")
        self.git("commit", "-q", "-m", message)

    def setUp(self):
        self.root = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.root)
        self.git("init", "-q", "-b", "staging")
        self.commit("version: 2\n", "base")
        self.git("checkout", "-q", "-b", "lane")
        self.git("checkout", "-q", "staging")
        self.commit("version: 3\n", "a bump lands on staging")
        self.git("update-ref", "refs/remotes/origin/staging", "staging")
        self.git("checkout", "-q", "lane")

    def test_a_branch_made_before_a_bump_reads_its_merge_base(self):
        self.assertEqual(journey.git_base_text("doc.md", self.root), "version: 2\n")

    def test_a_missing_ref_is_none(self):
        self.git("update-ref", "-d", "refs/remotes/origin/staging")
        self.assertIsNone(journey.git_base_text("doc.md", self.root))


class Targets(Tree):
    """The check that every id a journey names is one the app sets."""

    APP = "apps/mobile/Monaco/Screen.swift"

    def setUp(self):
        super().setUp()
        self.app("""struct Screen: View {
    var body: some View {
        Text("x").accessibilityIdentifier("login-submit")
        Text("y").accessibilityIdentifier("cabal-open-proposal-\\(proposal.id)")
        Text("z").accessibilityIdentifier(identifier)
        Text("c").accessibilityIdentifier("\\(identifier)-confirm")
    }
}
""")

    def app(self, text):
        self.write(self.APP, text)

    def doc(self, target, known=""):
        row = "| S1.1 | tap | %s | | b |\n" % target
        self.write("docs/journeys/auth/sign-in.md", DOC.replace("| S1.1 | tap | `a` | | b |\n", row) + known)

    def run_targets(self):
        return journey.check_targets(journey.load_journeys())

    def test_an_exact_id_the_app_sets_passes(self):
        self.doc("`login-submit`")
        errors, warnings, count = self.run_targets()
        self.assertEqual((errors, warnings, count), ([], [], 1))

    def test_an_interpolated_id_is_a_prefix(self):
        self.doc("`cabal-open-proposal-<proposalID>`")
        self.assertEqual(self.run_targets()[0], [])

    def test_a_composed_id_comes_from_the_tsv(self):
        self.doc("`profile-sign-out-confirm`")
        self.assertEqual(len(self.run_targets()[0]), 1)
        self.write("qa/ids-composed.tsv", "pattern\tfile\twhy\n<identifier>-confirm\t%s\ta dialog\n" % self.APP)
        self.assertEqual(self.run_targets()[0], [])

    def test_a_tsv_row_whose_file_builds_no_id_at_run_time_is_an_error(self):
        self.write("qa/ids-composed.tsv", "pattern\tfile\twhy\n<identifier>-confirm\t%s\ta dialog\n" % self.APP)
        self.doc("`login-submit`")
        self.assertEqual(self.run_targets()[0], [])
        self.app('Text("x").accessibilityIdentifier("login-submit")\n')
        errors = self.run_targets()[0]
        self.assertEqual(len(errors), 1)
        self.assertIn("ids-composed.tsv", errors[0])
        self.assertIn("builds its argument at run time", errors[0])

    def test_a_tsv_row_whose_fixed_text_is_gone_is_an_error(self):
        self.write("qa/ids-composed.tsv", "pattern\tfile\twhy\n<identifier>-gone\t%s\ta dialog\n" % self.APP)
        self.doc("`login-submit`")
        self.assertIn("matches nothing", self.run_targets()[0][0])

    def test_labels_in_quotes_and_system_controls_are_skipped(self):
        self.doc('"Sign in" and `BackButton`')
        errors, warnings, count = self.run_targets()
        self.assertEqual((errors, warnings, count), ([], [], 0))

    def test_an_id_the_text_says_is_absent_is_skipped(self):
        self.doc("`login-submit`, and there is no `login-gone`")
        self.assertEqual(self.run_targets()[0], [])

    def test_a_step_under_known_failures_is_a_warning(self):
        self.doc("`login-missing`", "\n## Known failures on staging\n\n- S1.1: gone. Blocked by #77.\n")
        errors, warnings, _ = self.run_targets()
        self.assertEqual(errors, [])
        self.assertEqual(len(warnings), 1)
        self.assertIn("#77", warnings[0])

    def test_a_miss_names_the_file_step_target_and_three_nearest_ids(self):
        self.app(self.app_text() + '        Text("a").accessibilityIdentifier("login-submit-all")\n'
                 '        Text("b").accessibilityIdentifier("login-subject")\n')
        self.doc("`login-submitt`")
        errors = self.run_targets()[0]
        self.assertEqual(len(errors), 1)
        self.assertTrue(errors[0].startswith("docs/journeys/auth/sign-in.md S1.1: target login-submitt is set nowhere in apps/mobile/Monaco (closest: "))
        nearest = errors[0].split("closest: ")[1].rstrip(")").split(", ")
        self.assertEqual(len(nearest), 3)
        self.assertEqual(nearest[0], "login-submit")

    def app_text(self):
        return (journey.ROOT / self.APP).read_text()

    def test_a_journey_test_that_looks_up_a_missing_id_is_named_with_its_step(self):
        self.write("ui/SignInJourney.swift", JOURNEY_SWIFT + 'let x = app.element("login-gone")\n'
                   'XCTAssertFalse(app.element("login-never").exists)\n')
        errors = self.run_targets()[0]
        self.assertEqual(len(errors), 1)
        self.assertIn("ui/SignInJourney.swift:6 (after step S1.1 in auth/sign-in): target login-gone is set nowhere", errors[0])


class Copy(Tree):
    """The check that the app can show each quote a journey doc makes and that tests quote their doc."""

    APP = "apps/mobile/Monaco/Copy.swift"
    DRIVER = "ui/SignInJourney.swift"

    def setUp(self):
        super().setUp()
        self.write(self.APP, 'Text("Welcome back")\nText("Voted \\(side). \\(ordinal) of \\(count).")\n')

    def doc(self, expect, known=""):
        row = "| S1.1 | tap | `a` | | %s |\n" % expect
        self.write("docs/journeys/auth/sign-in.md", DOC.replace("| S1.1 | tap | `a` | | b |\n", row) + known)

    def run_copy(self):
        return journey.check_copy(journey.load_journeys())

    def test_an_exact_app_string_passes(self):
        self.doc('"Welcome back"')
        self.assertEqual(self.run_copy(), ([], [], 1))

    def test_an_interpolated_literal_matches_by_its_fixed_parts(self):
        self.doc('"Voted Yes. 1st of 3."')
        self.assertEqual(self.run_copy()[0], [])
        self.doc('"Declined. 1st of 3."')
        self.assertEqual(len(self.run_copy()[0]), 1)

    def test_a_placeholder_matches_anything(self):
        self.doc('"Welcome {QA.run}"')
        self.assertEqual(self.run_copy()[0], [])

    def test_a_miss_names_the_doc_step_and_nearest_string(self):
        self.doc('"Welcome bak"')
        error = self.run_copy()[0][0]
        self.assertEqual(error, 'docs/journeys/auth/sign-in.md S1.1: "Welcome bak" is not app copy (closest: "Welcome back", Copy.swift:1)')

    def test_a_runtime_copy_row_makes_a_quote_producible(self):
        self.doc('"QA vote {QA.run}"')
        self.assertEqual(len(self.run_copy()[0]), 1)
        self.write("qa/copy-runtime.tsv", "copy\tsource\twhy\nQA vote {QA.run}\tsetup.sh\ta seeded title\n")
        self.assertEqual(self.run_copy()[0], [])

    def test_a_step_under_known_failures_is_a_warning(self):
        self.doc('"Gone"', "\n## Known failures on staging\n\n- S1.1: gone. Blocked by #77.\n")
        errors, warnings, _ = self.run_copy()
        self.assertEqual(errors, [])
        self.assertEqual(len(warnings), 1)
        self.assertIn("#77", warnings[0])

    def test_a_test_literal_missing_from_its_doc_step_is_an_error(self):
        self.doc('"Welcome back"')
        self.write(self.DRIVER, JOURNEY_SWIFT + 'expectToast(app, "Welcome back")\nexpectToast(app, "Welcome home")\n')
        errors = self.run_copy()[0]
        self.assertEqual(len(errors), 1)
        self.assertEqual(errors[0], 'ui/SignInJourney.swift:7 S1.1: "Welcome home" is not in docs/journeys/auth/sign-in.md S1.1')


class SetupInserts(Tree):
    def setUp(self):
        super().setUp()
        self.write("apps/backend/migrations/20260101000000_widgets.sql", MIGRATIONS)

    def insert(self, columns):
        self.write(
            "apps/mobile/qa/journeys/x/y.setup.sh",
            "psql <<SQL\nINSERT INTO widgets (%s)\nVALUES (1);\nSQL\n" % columns)
        return journey.check_setup_inserts()

    def test_an_insert_that_omits_a_required_column_is_named(self):
        problems = self.insert("id, name, colour")
        self.assertEqual(len(problems), 1)
        self.assertIn("apps/mobile/qa/journeys/x/y.setup.sh", problems[0])
        self.assertIn("widgets", problems[0])
        self.assertIn("size", problems[0])

    def test_an_insert_that_omits_a_primary_key_without_a_default_is_named(self):
        problems = self.insert("name, size, colour")
        self.assertEqual(len(problems), 1)
        self.assertIn("column id", problems[0])

    def test_a_table_level_primary_key_makes_its_columns_required(self):
        self.write("apps/backend/migrations/20260102000000_pairs.sql",
                   "CREATE TABLE pairs (a int, b int, PRIMARY KEY (a, b));\n")
        self.write("apps/mobile/qa/journeys/x/z.setup.sh", "INSERT INTO pairs (a)\nVALUES (1);\n")
        problems = journey.check_setup_inserts()
        self.assertEqual(len(problems), 1)
        self.assertIn("column b", problems[0])

    def test_an_insert_that_omits_only_nullable_or_defaulted_columns_passes(self):
        self.assertEqual(self.insert("id, name, size, colour"), [])

    def test_an_insert_without_a_column_list_is_skipped(self):
        self.write("apps/mobile/qa/journeys/x/y.truth.sh", "INSERT INTO widgets VALUES (1);\n")
        self.assertEqual(journey.check_setup_inserts(), [])


class RealSetupInserts(unittest.TestCase):
    def test_every_setup_insert_in_the_tree_names_its_required_columns(self):
        self.assertEqual(journey.check_setup_inserts(), [])


class SetupMonacoctl(Tree):
    def test_a_setup_that_calls_bin_monacoctl_directly_is_named(self):
        self.write("qa/auth/sign-in.setup.sh", "token=$(env -u FOO bin/monacoctl dev token --user new)\n")
        problems = journey.check_setup_monacoctl()
        self.assertEqual(len(problems), 1)
        self.assertIn("qa/auth/sign-in.setup.sh:1: runs monacoctl itself", problems[0])

    def test_a_truth_script_that_runs_it_through_with_dotenv_is_named(self):
        self.write("qa/auth/sign-in.truth.sh", "scripts/with-dotenv-local.sh bin/monacoctl dev token\n")
        self.assertEqual(len(journey.check_setup_monacoctl()), 1)

    def test_a_call_through_a_variable_path_or_go_run_is_named(self):
        self.write("qa/auth/sign-in.setup.sh", '"$root/bin/monacoctl" dev token\n./bin/monacoctl dev token\n')
        self.write("qa/auth/sign-in.truth.sh", "go run ./cmd/monacoctl dev token\n")
        self.assertEqual(len(journey.check_setup_monacoctl()), 3)

    def test_the_wrapper_and_a_comment_pass(self):
        self.write(
            "qa/auth/sign-in.setup.sh",
            "# not bin/monacoctl itself\ntoken=$(scripts/qa/monacoctl.sh dev token --user new)\n",
        )
        self.assertEqual(journey.check_setup_monacoctl(), [])

    def test_the_checked_in_scripts_pass(self):
        journey.ROOT, journey.QA = self.saved[0], self.saved[2]
        self.assertEqual(journey.check_setup_monacoctl(), [])


class Coverage(Tree):
    def test_lists_backend_and_app_statuses_and_uncovered_flows(self):
        self.write("packages/flows/app/01.tsv", "id\tscreen\tstatus\n01\tSignIn\tplanned\n")
        self.write("packages/flows/backend/02.tsv", "id\tflow\tstatus\n02\tSign out\tplanned\n")
        output = StringIO()
        with redirect_stdout(output):
            self.assertEqual(journey.cmd_coverage(None), 0)
        self.assertIn("| 01 | Sign in | built | planned | auth/sign-in |", output.getvalue())
        self.assertIn("| 02 | Sign out | planned | - | - |", output.getvalue())
        self.assertIn("uncovered: 02", output.getvalue())


class SimslimProfile(Tree):
    def declare(self, name):
        self.write("docs/journeys/auth/sign-in.md", DOC.replace("actors: [A]\n", "actors: [A]\nsimslim: %s\n" % name))

    def test_a_journey_without_simslim_has_no_profile(self):
        self.assertEqual(journey.load_journeys()["auth/sign-in"].simslim, "")

    def test_a_named_profile_that_exists_passes_and_is_read(self):
        self.write("ci/profiles/photos.json", "{}")
        self.declare("photos")
        self.assertEqual(self.problems(), [])
        self.assertEqual(journey.load_journeys()["auth/sign-in"].simslim, "photos")

    def test_an_unknown_profile_is_named(self):
        self.declare("nope")
        self.assertIn("simslim names profile 'nope', which has no ci/profiles/nope.json", self.problems()[0])


class FakeRpc(Tree):
    def declare(self, value):
        self.write("docs/journeys/auth/sign-in.md", DOC.replace("actors: [A]\n", "actors: [A]\nrpc: %s\n" % value))

    def args(self):
        return unittest.mock.Mock(**{"all": False, "journey": ["auth/sign-in"]})

    def test_a_journey_without_rpc_reads_the_fakes(self):
        self.assertEqual(journey.load_journeys()["auth/sign-in"].rpc, "")
        self.assertTrue(journey.backend_fake_rpc(self.args()))

    def test_rpc_fake_passes_and_asks_for_the_fakes(self):
        self.declare("fake")
        self.assertEqual(self.problems(), [])
        self.assertTrue(journey.backend_fake_rpc(self.args()))

    def test_rpc_fake_trade_passes_asks_for_the_fakes_and_for_fake_trading(self):
        self.declare("fake-trade")
        self.assertEqual(self.problems(), [])
        self.assertTrue(journey.backend_fake_rpc(self.args()))
        self.assertTrue(journey.backend_fake_trade(self.args()))

    def test_rpc_fake_and_a_plain_journey_do_not_ask_for_fake_trading(self):
        self.assertFalse(journey.backend_fake_trade(self.args()))
        self.declare("fake")
        self.assertFalse(journey.backend_fake_trade(self.args()))

    def test_rpc_fake_trade_with_funds_is_refused(self):
        self.write("docs/journeys/auth/sign-in.md",
                   DOC.replace("actors: [A]\n", "actors: [A]\nrpc: fake-trade\nfunds:\n  A: 2\n"))
        self.assertIn("rpc: fake-trade and funds cannot both be set", self.problems()[0])

    def test_rpc_live_passes_and_keeps_the_real_endpoint(self):
        self.declare("live")
        self.assertEqual(self.problems(), [])
        self.assertFalse(journey.backend_fake_rpc(self.args()))

    def test_another_rpc_value_is_named(self):
        self.declare("mainnet")
        self.assertIn("rpc is 'mainnet': the values are fake, fake-trade and live", self.problems()[0])

    def test_rpc_fake_with_funds_is_refused(self):
        self.write("docs/journeys/auth/sign-in.md",
                   DOC.replace("actors: [A]\n", "actors: [A]\nrpc: fake\nfunds:\n  A: 2\n"))
        self.assertIn("rpc: fake and funds cannot both be set", self.problems()[0])

    def test_a_journey_with_funds_keeps_the_real_endpoint_without_declaring_anything(self):
        self.write("docs/journeys/auth/sign-in.md", DOC.replace("actors: [A]\n", "actors: [A]\nfunds:\n  A: 2\n"))
        self.assertEqual(self.problems(), [])
        self.assertFalse(journey.backend_fake_rpc(self.args()))

    def test_one_live_or_funded_journey_keeps_the_real_endpoint_for_the_whole_run(self):
        plain = unittest.mock.Mock(rpc="", funds={})
        fake = unittest.mock.Mock(rpc="fake", funds={})
        live = unittest.mock.Mock(rpc="live", funds={})
        funded = unittest.mock.Mock(rpc="", funds={"A": 2})
        run = lambda *names: unittest.mock.Mock(**{"all": False, "journey": list(names)})
        with unittest.mock.patch.object(
                journey, "load_journeys", lambda: {"p": plain, "f": fake, "l": live, "m": funded}):
            self.assertTrue(journey.backend_fake_rpc(run("p", "f")))
            self.assertFalse(journey.backend_fake_rpc(run("p", "l")))
            self.assertFalse(journey.backend_fake_rpc(run("p", "m")))
            self.assertFalse(journey.backend_fake_rpc(unittest.mock.Mock(**{"all": True, "journey": []})))


class FakeRpcSeeds(unittest.TestCase):
    def test_the_sell_journey_puts_its_treasury_tokens_on_the_fake(self):
        root = journey.ROOT
        setup = (root / "apps/mobile/qa/journeys/governance/propose-sell.setup.sh").read_text()
        seed = (root / "scripts/qa/seed.sh").read_text()
        self.assertIn('qa_fake_treasury_positions "$CABAL_ID"', setup)
        self.assertIn("qa_fake_treasury_positions() {", seed)
        self.assertIn("FROM cabal_positions p", seed)

    def test_a_mutants_pass_follows_the_journeys_rpc_choice(self):
        self.assertTrue(journey.journey_fake_rpc(unittest.mock.Mock(rpc="", funds={})))
        self.assertFalse(journey.journey_fake_rpc(unittest.mock.Mock(rpc="live", funds={})))
        self.assertFalse(journey.journey_fake_rpc(unittest.mock.Mock(rpc="", funds={"A": 1})))


class FakeRpcEnvironment(unittest.TestCase):
    def test_the_run_points_at_its_own_fakes_and_puts_the_environment_back(self):
        started, stopped = [], []
        environ = {"QA_FAKES_URL": "http://old"}
        with unittest.mock.patch.dict(os.environ, environ, clear=True), \
                unittest.mock.patch.object(journey, "start_fakes", lambda slot: started.append(slot) or "proc"), \
                unittest.mock.patch.object(journey, "stop_backend", stopped.append):
            with journey.fake_rpc_environment(1):
                self.assertEqual(os.environ["QA_FAKE_RPC"], "1")
                self.assertEqual(os.environ["QA_FAKES_URL"], "http://127.0.0.1:8199")
            self.assertNotIn("QA_FAKE_RPC", os.environ)
            self.assertEqual(os.environ["QA_FAKES_URL"], "http://old")
        self.assertEqual((started, stopped), ([1], ["proc"]))

    def test_fake_trade_is_set_for_the_run_and_put_back(self):
        with unittest.mock.patch.dict(os.environ, {}, clear=True), \
                unittest.mock.patch.object(journey, "start_fakes", lambda slot: "proc"), \
                unittest.mock.patch.object(journey, "stop_backend", lambda process: None):
            with journey.fake_rpc_environment(0, True):
                self.assertEqual(os.environ["QA_FAKE_TRADE"], "1")
            self.assertNotIn("QA_FAKE_TRADE", os.environ)

    def test_a_caller_with_its_own_fakes_keeps_control(self):
        with unittest.mock.patch.dict(os.environ, {"QA_FAKE_RPC": "1"}, clear=True), \
                unittest.mock.patch.object(journey, "start_fakes", lambda slot: self.fail("started fakes")):
            with journey.fake_rpc_environment(0):
                self.assertEqual(os.environ["QA_FAKE_RPC"], "1")

    def test_the_fakes_start_on_the_slot_port_and_stop_when_they_do_not_listen(self):
        calls = []
        with tempfile.TemporaryDirectory() as out, \
                unittest.mock.patch.object(journey, "OUT", Path(out)), \
                unittest.mock.patch.object(journey, "listeners", lambda ports=None, run=None: []), \
                unittest.mock.patch.object(journey, "sh", lambda args, **k: calls.append(args)), \
                unittest.mock.patch.object(journey.subprocess, "Popen", lambda *a, **k: unittest.mock.Mock(poll=lambda: 1)), \
                unittest.mock.patch.object(journey, "stop_backend", lambda process: calls.append("stopped")), \
                redirect_stdout(StringIO()):
            with self.assertRaisesRegex(journey.JourneyError, "did not listen on :8099"):
                journey.start_fakes(0)
        self.assertEqual(calls[0][:5], ["go", "-C", "apps/backend", "build", "-o"])
        self.assertEqual(calls[-1], "stopped")

    def test_the_fakes_return_once_the_port_listens(self):
        process = unittest.mock.Mock(poll=lambda: None)
        with tempfile.TemporaryDirectory() as out, \
                unittest.mock.patch.object(journey, "OUT", Path(out)), \
                unittest.mock.patch.object(journey, "listeners", unittest.mock.Mock(side_effect=[[], [("1", "/")]])), \
                unittest.mock.patch.object(journey, "sh", lambda args, **k: None), \
                unittest.mock.patch.object(journey.subprocess, "Popen", lambda *a, **k: process), \
                unittest.mock.patch.object(journey.time, "sleep", lambda s: None), \
                redirect_stdout(StringIO()):
            self.assertIs(journey.start_fakes(0), process)


class Preconditions(Tree):
    def declare(self, lines, funds=""):
        self.write("docs/journeys/auth/sign-in.md", DOC.replace(
            "actors: [A]\n", "actors: [A]\n%spreconditions:\n%s" % (funds, lines)))
        self.write("qa/auth/sign-in.setup.sh", "#!/bin/bash\n")
        self.write("qa/fixtures/photo.jpg", "x")
        return journey.load_journeys()["auth/sign-in"]

    def test_the_keys_parse_and_funds_implies_a_balance(self):
        loaded = self.declare("  balance.A: 3\n  photos: 1\n  fresh_handle: true\n", funds="funds:\n  A: 2\n")
        self.assertEqual(loaded.preconditions, {"balance.A": 3, "photos": 1, "fresh_handle": "true"})
        self.assertEqual(loaded.balance_needs(), {"A": 3})
        self.assertEqual(self.problems(), [])
        plain = self.declare("  photos: 1\n", funds="funds:\n  A: 2\n")
        self.assertEqual(plain.balance_needs(), {"A": 2})

    def test_check_names_a_bad_key_and_a_missing_fixture(self):
        self.declare("  balance.Z: 1\n  sparkles: 2\n")
        self.assertEqual(len([p for p in self.problems() if "preconditions" in p]), 2)
        self.declare("  photos: 1\n")
        (journey.QA / "fixtures" / "photo.jpg").unlink()
        self.assertTrue(any("photo.jpg" in p for p in self.problems()))

    def test_a_cash_shortfall_names_the_fix_and_a_funded_actor_passes(self):
        loaded = self.declare("  balance.A: 1\n")
        poor = journey.check_preconditions(loaded, {}, "http://x", balance=lambda *a: ("u-1", 0))
        self.assertEqual(poor, ["A's cash is $0.00, under the $1 auth/sign-in needs: run scripts/qa/monacoctl.sh "
                                "qa fund --user u-1 --usdc 1"])
        self.assertEqual(journey.check_preconditions(loaded, {}, "http://x", balance=lambda *a: ("u-1", 1500000)), [])

    def test_a_foreign_handle_holder_is_a_problem(self):
        loaded = self.declare("  fresh_handle: true\n")
        problems = journey.check_preconditions(loaded, {}, "http://x", handle=lambda *a: "taken by someone")
        self.assertEqual(problems, ["the handle auth/sign-in needs is taken: taken by someone"])

    def test_a_failed_precondition_is_setup_exit_2_and_never_builds(self):
        self.declare("  balance.A: 1\n")
        built, rows, booted = [], [], []
        args = types.SimpleNamespace(sim="", channel="sms", runs=1, fresh=False, timeout=5)
        loaded = journey.load_journeys()["auth/sign-in"]
        with unittest.mock.patch.object(journey, "held_logins", lambda j, a: []), \
                unittest.mock.patch.object(journey, "use_derived_data", lambda l: None), \
                unittest.mock.patch.object(journey, "resolve_simulator", lambda *a: booted.append("resolve") or "sim"), \
                unittest.mock.patch.object(journey, "check_simulator_api_environment",
                                           lambda s, u: booted.append("boot")), \
                unittest.mock.patch.object(journey, "actor_balance", lambda *a: ("u-1", 0)), \
                unittest.mock.patch.object(journey, "record", lambda *a, **k: rows.append(a[4])), \
                unittest.mock.patch.object(journey, "OUT", Path(self.tmp.name) / "out"), \
                redirect_stdout(StringIO()) as printed:
            code = journey.run_journey_as(args, "http://x", {}, loaded, ["S1"], {}, lambda sim: built.append(sim))
        self.assertEqual((code, built, booted), (2, [], []))
        self.assertEqual(rows[0]["result"], "SETUP")
        self.assertIn("SETUP auth/sign-in: A's cash is $0.00", printed.getvalue())

    def test_report_counts_setup_apart_from_fail(self):
        base = {"journey": "j", "driver": journey.DRIVER, "build": "b", "scenario": "*", "truth": ""}
        table = journey.summarize([dict(base, result="SETUP"), dict(base, result="FAIL"), dict(base, result="ERROR")])
        self.assertEqual((table[0]["setup"], table[0]["errors"], table[0]["runs"]), (1, 1, 1))


class Funds(Tree):
    def fund(self, lines):
        self.write("docs/journeys/auth/sign-in.md", DOC.replace("actors: [A]\n", "actors: [A]\nfunds:\n" + lines))

    def test_a_journey_without_funds_has_no_notice(self):
        self.assertEqual(journey.funding_notice(journey.load_journeys()["auth/sign-in"], {}), "")

    def test_a_money_journey_offers_the_qa_pot_and_the_phantom_wallet_per_actor(self):
        self.fund("  A: 2\n")
        self.assertEqual(self.problems(), [])
        accounts = {"A": {"privy_user_id": "did:privy:abc"}}
        notice = journey.funding_notice(journey.load_journeys()["auth/sign-in"], accounts,
                                        user_id=lambda row: "uuid-" + row["privy_user_id"][-3:])
        self.assertIn("bin/monacoctl qa fund --user uuid-abc --usdc 2", notice)
        self.assertIn("or send 2 USDC from the Phantom agent wallet to A's deposit address", notice)
        before_sign_in = journey.funding_notice(journey.load_journeys()["auth/sign-in"], accounts,
                                                user_id=lambda row: "")
        self.assertIn("qa fund --user <A's user id, after its first sign-in> --usdc 2", before_sign_in)

    def test_the_user_id_lookup_quotes_only_a_privy_id(self):
        with unittest.mock.patch.object(journey, "with_dotenv", lambda args: "never"):
            self.assertEqual(journey.actor_user_id({"privy_user_id": "x'; DROP TABLE users; --"}), "")
        seen = []
        with unittest.mock.patch.object(journey, "with_dotenv", lambda args: seen.append(args) or "u-1"):
            self.assertEqual(journey.actor_user_id({"privy_user_id": "did:privy:abc"}), "u-1")
        self.assertIn("SELECT id FROM users WHERE privy_user_id = 'did:privy:abc'", seen[0])

    def test_funds_for_an_unknown_actor_or_a_bad_amount_are_named(self):
        self.fund("  B: 2\n  A: lots\n")
        problems = "\n".join(self.problems())
        self.assertIn("funds names actor B, which is not in actors", problems)
        self.assertIn("funds for actor A must be whole USDC from 1, got 'lots'", problems)

    def test_an_exported_phantom_refund_address_wins_over_the_pot(self):
        asked = []
        with unittest.mock.patch.dict(os.environ, {"MONACO_QA_REFUND_ADDRESS": "Phantom1"}), \
                unittest.mock.patch.object(journey, "with_dotenv", lambda args: asked.append(args) or "Pot1"):
            self.assertEqual(journey.refund_address(), "Phantom1")
            env = journey.actor_environment({}, "sms", "RUN123", prefix="TEST_RUNNER_")
        self.assertEqual(env["TEST_RUNNER_MONACO_QA_REFUND_ADDRESS"], "Phantom1")
        self.assertEqual(asked, [])

    def test_without_an_export_the_refund_address_is_the_qa_pot(self):
        asked = []
        with unittest.mock.patch.dict(os.environ, {"MONACO_QA_REFUND_ADDRESS": ""}), \
                unittest.mock.patch.object(journey, "with_dotenv", lambda args: asked.append(args) or "Pot1"):
            self.assertEqual(journey.refund_address(), "Pot1")
            env = journey.actor_environment({}, "sms", "RUN123", prefix="TEST_RUNNER_")
        self.assertEqual(env["TEST_RUNNER_MONACO_QA_REFUND_ADDRESS"], "Pot1")
        self.assertEqual(asked[0][-3:], ["qa", "pot", "--address"])

    def test_a_money_run_refuses_to_start_with_no_refund_address(self):
        with unittest.mock.patch.dict(os.environ, {"MONACO_QA_REFUND_ADDRESS": ""}), \
                unittest.mock.patch.object(journey, "with_dotenv", lambda args: ""):
            with self.assertRaisesRegex(journey.JourneyError, "monacoctl qa pot --address"):
                journey.refund_address()


class Composition(Tree):
    def add(self, journey_id, requires):
        self.write("docs/journeys/%s.md" % journey_id, DOC.replace("id: auth/sign-in", "id: " + journey_id)
                   .replace("requires: []", "requires: [%s]" % ", ".join(requires)))

    def test_a_cycle_is_named(self):
        self.add("cabal/create", ["cabal/join"])
        self.add("cabal/join", ["cabal/create"])
        self.assertTrue(any("requires form a cycle" in p for p in self.problems()))


class Accounts(Tree):
    def test_the_environment_takes_over_a_row(self):
        accounts = journey.load_accounts(environ={"MONACO_QA_A_CODE": "654321"})
        self.assertEqual(accounts["A"]["code"], "654321")
        self.assertEqual(accounts["A"]["phone"], "555")


class QALock(unittest.TestCase):
    def setUp(self):
        folder = tempfile.TemporaryDirectory()
        self.addCleanup(folder.cleanup)
        self.path = str(Path(folder.name) / "qa.lock")

    def held_elsewhere(self):
        handle = open(self.path, "a")
        try:
            fcntl.flock(handle, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            handle.close()
            return True
        handle.close()
        return False

    def test_a_run_holds_the_lock_until_it_lets_go(self):
        handle = journey.take_qa_lock(self.path)
        self.assertTrue(self.held_elsewhere())
        handle.close()
        self.assertFalse(self.held_elsewhere())

    def test_a_second_run_waits_for_the_first(self):
        first = journey.take_qa_lock(self.path)
        started = time.monotonic()
        threading.Timer(0.3, first.close).start()
        second = journey.take_qa_lock(self.path)
        self.addCleanup(second.close)
        self.assertGreaterEqual(time.monotonic() - started, 0.25)



class BusyPorts(unittest.TestCase):
    def lsof(self, answers):
        calls = []

        def run(args, **kwargs):
            calls.append(args)
            return type("Result", (), {"stdout": answers.get(args[-1], answers.get(args[3], ""))})()

        return run, calls

    def test_each_listener_is_named_with_its_worktree(self):
        run, _ = self.lsof({"-iTCP:8081": "55430\n55429\n55430\n", "55429": "p55429\nfcwd\nn/w/644\n",
                            "55430": "p55430\nfcwd\nn/w/644\n"})
        busy = journey.listeners(run=run)
        self.assertEqual(busy, [("55429", "/w/644"), ("55430", "/w/644")])
        with self.assertRaisesRegex(journey.JourneyError,
                                    r"port 8080 or 8081 is already in use by pid 55429 \(/w/644\), pid 55430 \(/w/644\)"):
            journey.refuse_busy_ports(busy)

    def test_free_ports_let_the_run_start_its_own_backend(self):
        run, calls = self.lsof({})
        self.assertEqual(journey.listeners(run=run), [])
        self.assertEqual(len(calls), 1)
        journey.refuse_busy_ports([])

    def test_a_busy_port_stops_the_run_before_it_starts_a_backend(self):
        saved = journey.listeners, journey.start_backend
        folder = tempfile.TemporaryDirectory()
        self.addCleanup(folder.cleanup)

        def started(base_url, timeout=300, slot=0):
            self.fail("started a backend while another one listens")

        journey.listeners = lambda ports=None: [("55430", "/w/644")]
        journey.start_backend = started
        self.addCleanup(lambda: setattr(journey, "listeners", saved[0]))
        self.addCleanup(lambda: setattr(journey, "start_backend", saved[1]))
        environ = dict(os.environ)
        environ.pop("MONACO_API_BASE_URL", None)
        with unittest.mock.patch.dict(os.environ, environ, clear=True):
            with self.assertRaisesRegex(journey.JourneyError, "pid 55430 \\(/w/644\\)"):
                with journey.journey_backend():
                    self.fail("the run went ahead")


class JourneyTradeEngine(unittest.TestCase):
    """The backend a run starts stubs the trade engine, except for a run that moves real USDC."""

    def args(self, **values):
        return unittest.mock.Mock(**{"all": False, "journey": ["money/fund-cabal"], **values})

    def pick(self, journey_id, environ):
        funded = unittest.mock.Mock(funds={"A": 2})
        free = unittest.mock.Mock(funds={})
        journeys = {"money/fund-cabal": funded, "auth/sign-in": free}
        with unittest.mock.patch.object(journey, "load_journeys", lambda: journeys), \
                unittest.mock.patch.dict(os.environ, environ, clear=True):
            return journey.backend_trade_engine(self.args(journey=[journey_id]))

    def test_a_journey_without_funds_runs_against_the_stub(self):
        self.assertEqual(self.pick("auth/sign-in", {}), "stub")

    def test_a_journey_with_funds_runs_against_the_live_engine(self):
        self.assertEqual(self.pick("money/fund-cabal", {}), "live")

    def test_all_stubs_unless_the_funded_journeys_will_run(self):
        journeys = {"money/fund-cabal": unittest.mock.Mock(funds={"A": 2})}
        for environ, want in (({}, "stub"), ({"MONACO_QA_REFUND_ADDRESS": "wallet"}, "live")):
            with unittest.mock.patch.object(journey, "load_journeys", lambda: journeys), \
                    unittest.mock.patch.dict(os.environ, environ, clear=True):
                self.assertEqual(journey.backend_trade_engine(self.args(all=True)), want)

    def test_start_backend_keeps_a_trade_engine_the_caller_set(self):
        for environ, want in (({}, "stub"), ({"TRADE_ENGINE": "live"}, "live")):
            seen = {}

            def popen(_args, **kwargs):
                seen.update(kwargs["env"])
                raise RuntimeError("stop")

            with unittest.mock.patch.dict(os.environ, environ, clear=True), \
                    unittest.mock.patch.object(journey, "apply_event_streams", lambda log, url=None: None), \
                    unittest.mock.patch.object(journey.subprocess, "Popen", popen), \
                    unittest.mock.patch.object(journey, "OUT", Path(tempfile.mkdtemp())):
                with self.assertRaises(RuntimeError):
                    journey.start_backend("http://127.0.0.1:1")
            self.assertEqual(seen["TRADE_ENGINE"], want)

    def test_start_backend_does_not_start_a_fund_page(self):
        seen = {}

        def popen(_args, **kwargs):
            seen.update(kwargs["env"])
            raise RuntimeError("stop")

        with unittest.mock.patch.dict(os.environ, {"MONACO_FUND_PAGE_PORT": "5173"}, clear=True), \
                unittest.mock.patch.object(journey, "apply_event_streams", lambda log, url=None: None), \
                unittest.mock.patch.object(journey.subprocess, "Popen", popen), \
                unittest.mock.patch.object(journey, "OUT", Path(tempfile.mkdtemp())):
            with self.assertRaises(RuntimeError):
                journey.start_backend("http://127.0.0.1:1")
        self.assertEqual(seen["MONACO_FUND_PAGE_PORT"], "off")

    def test_backend_commands_get_no_runner_only_variables(self):
        runner_only = {"MONACO_NO_SIMSLIM": "1", "MONACO_LOCK_HOLD": "5", "MONACO_QA_REFUND_ADDRESS": "wallet"}
        seen = []

        def run(step, **kwargs):
            seen.append(kwargs["env"])
            return types.SimpleNamespace(returncode=0)

        with unittest.mock.patch.dict(os.environ, dict(runner_only, TRADE_ENGINE="stub"), clear=True), \
                unittest.mock.patch.object(journey, "sh", run):
            journey.apply_event_streams(Path(tempfile.mkdtemp()) / "backend.log")

            def popen(_args, **kwargs):
                seen.append(kwargs["env"])
                raise RuntimeError("stop")

            with unittest.mock.patch.object(journey, "apply_event_streams", lambda log, url=None: None), \
                    unittest.mock.patch.object(journey.subprocess, "Popen", popen), \
                    unittest.mock.patch.object(journey, "OUT", Path(tempfile.mkdtemp())):
                with self.assertRaises(RuntimeError):
                    journey.start_backend("http://127.0.0.1:1")
        self.assertEqual(len(seen), 4)
        for env in seen:
            self.assertEqual(env["TRADE_ENGINE"], "stub")
            for key in runner_only:
                self.assertNotIn(key, env)


class RunTimeout(unittest.TestCase):
    def journey(self, timeout):
        return types.SimpleNamespace(timeout=timeout)

    def test_the_flag_beats_the_doc_and_the_doc_beats_the_default(self):
        flag = types.SimpleNamespace(timeout=900)
        none = types.SimpleNamespace(timeout=None)
        self.assertEqual(journey.run_timeout(flag, self.journey(600)), 900)
        self.assertEqual(journey.run_timeout(none, self.journey(600)), 600)
        self.assertEqual(journey.run_timeout(none, self.journey(None)), journey.DEFAULT_RUN_TIMEOUT)
        self.assertEqual(journey.run_timeout(none, self.journey("600")), journey.DEFAULT_RUN_TIMEOUT)

    def test_cabals_join_declares_room_for_its_three_scenarios(self):
        join = journey.pick(journey.load_journeys(), "cabals/join")
        self.assertGreater(join.timeout, journey.DEFAULT_RUN_TIMEOUT)


class Slots(unittest.TestCase):
    def setUp(self):
        folder = tempfile.TemporaryDirectory()
        self.addCleanup(folder.cleanup)
        for name, template in (("SLOT_LOCK", "/slot%d.lock"), ("ACTOR_LOCK", "/actor-%s.lock")):
            patcher = unittest.mock.patch.object(journey, name, folder.name + template)
            patcher.start()
            self.addCleanup(patcher.stop)
        self.held = []
        self.addCleanup(lambda: [handle.close() for handle in self.held])

    def hold(self, *handles):
        self.held.extend(handle[1] if isinstance(handle, tuple) else handle for handle in handles)
        return handles

    def test_two_runs_get_different_slots_and_ports(self):
        first, second = self.hold(journey.take_slot(), journey.take_slot())
        self.assertEqual((first[0], second[0]), (0, 1))
        self.assertEqual(journey.slot_base_url(0), "http://127.0.0.1:8080")
        self.assertEqual(journey.slot_base_url(1), "http://127.0.0.1:8180")
        self.assertEqual(journey.SLOT_PORTS, ((8080, 8081), (8180, 8181), (8280, 8281)))
        self.assertEqual(len(journey.FAKES_PORTS), len(journey.SLOT_PORTS))
        self.assertEqual(len(journey.NATS_PORTS), len(journey.SLOT_PORTS))

    def test_a_run_waits_when_every_slot_is_taken(self):
        first, *_ = self.hold(*(journey.take_slot() for _ in journey.SLOT_PORTS))
        waits = []

        def sleep(seconds):
            waits.append(seconds)
            first[1].close()

        with redirect_stdout(StringIO()):
            third = self.hold(journey.take_slot(sleep=sleep))[0]
        self.assertEqual(waits, [journey.LOCK_POLL_SECONDS])
        self.assertEqual(third[0], 0)

    def test_slot_forces_a_slot(self):
        slot, _ = self.hold(journey.take_slot(1))[0]
        self.assertEqual(slot, 1)
        free = journey.try_lock(journey.SLOT_LOCK % 0)
        self.assertIsNotNone(free)
        free.close()
        self.assertIsNone(journey.try_lock(journey.SLOT_LOCK % 1))
        with self.assertRaisesRegex(journey.JourneyError, "--slot is 0 to 2"):
            journey.take_slot(3)

    def test_a_slot_starts_its_backend_on_its_own_ports(self):
        popen = []
        with unittest.mock.patch.object(journey.subprocess, "Popen", lambda *a, **k: popen.append(k["env"]) or
                                        type("P", (), {"poll": lambda self: None})()), \
                unittest.mock.patch.object(journey, "OUT", Path(tempfile.mkdtemp())), \
                unittest.mock.patch.object(journey, "apply_event_streams", lambda log, url=None: None), \
                unittest.mock.patch.object(journey, "backend_is_running", lambda url: True):
            journey.start_backend("http://127.0.0.1:8180", slot=1)
        self.assertEqual(popen[0]["MONACO_HTTP_ADDR"], ":8180")
        self.assertEqual(popen[0]["MONACO_WORKER_HEALTH_ADDR"], ":8181")
        self.assertRegex(popen[0]["MONACO_LOG_DIR"], r"backend-slot1-\d{8}T\d{6}Z$")

    def test_each_slot_backend_gets_its_own_nats(self):
        popen, applied = [], []
        with unittest.mock.patch.object(journey.subprocess, "Popen", lambda *a, **k: popen.append(k["env"]) or
                                        type("P", (), {"poll": lambda self: None})()), \
                unittest.mock.patch.object(journey, "OUT", Path(tempfile.mkdtemp())), \
                unittest.mock.patch.object(journey, "apply_event_streams", lambda log, url=None: applied.append(url)), \
                unittest.mock.patch.object(journey, "backend_is_running", lambda url: True), \
                unittest.mock.patch.dict(os.environ, {"NATS_URL": "nats://127.0.0.1:4222"}):
            for slot in (0, 1):
                journey.start_backend(journey.slot_base_url(slot), slot=slot)
        urls = [env["NATS_URL"] for env in popen]
        self.assertEqual(urls, ["nats://127.0.0.1:4232", "nats://127.0.0.1:4233"])
        self.assertEqual(applied, urls)

    def test_a_slot_nats_container_is_removed_on_stop(self):
        calls = []

        def run(args, **kwargs):
            calls.append(args)
            return types.SimpleNamespace(returncode=0)

        with unittest.mock.patch.object(journey, "sh", run), \
                unittest.mock.patch.object(journey, "listeners", lambda ports=None: []), \
                unittest.mock.patch.object(journey.socket, "create_connection", lambda *a, **k: socket.socket()):
            with journey.slot_nats(1) as url:
                self.assertEqual(url, "nats://127.0.0.1:4233")
                self.assertEqual(calls[-1][:5], ["docker", "run", "-d", "--name", "monaco-qa-nats-slot1"])
                self.assertIn("127.0.0.1:4233:4222", calls[-1])
            self.assertEqual(calls[-1], ["docker", "rm", "-f", "monaco-qa-nats-slot1"])
            self.assertEqual(calls[0], ["docker", "rm", "-f", "monaco-qa-nats-slot1"])

    def test_a_slot_backend_leaves_price_history_off(self):
        popen = []
        with unittest.mock.patch.object(journey.subprocess, "Popen", lambda *a, **k: popen.append(k["env"]) or
                                        type("P", (), {"poll": lambda self: None})()), \
                unittest.mock.patch.object(journey, "OUT", Path(tempfile.mkdtemp())), \
                unittest.mock.patch.object(journey, "apply_event_streams", lambda log, url=None: None), \
                unittest.mock.patch.object(journey, "backend_is_running", lambda url: True), \
                unittest.mock.patch.dict(os.environ, {"MARKET_HISTORY_ENABLED": "true"}):
            for slot in (0, 1):
                journey.start_backend(journey.slot_base_url(slot), slot=slot)
        self.assertEqual([env["MARKET_HISTORY_ENABLED"] for env in popen], ["false", "false"])

    def test_the_same_login_does_not_run_twice(self):
        logins = ["A", "B", "C", "L"]
        mapping, handles = journey.claim_logins(["A", "B"], logins)
        self.hold(*handles)
        self.assertEqual(mapping, {"A": "A", "B": "B"})
        self.assertIsNone(journey.try_lock(journey.ACTOR_LOCK % "A"))

    def test_a_run_in_slot_one_takes_the_logins_slot_zero_leaves(self):
        logins = ["A", "B", "C", "L"]
        self.hold(*journey.claim_logins(["A", "B"], logins)[1])
        mapping, handles = journey.claim_logins(["A"], logins)
        self.hold(*handles)
        self.assertEqual(mapping, {"A": "C"})

    def test_a_run_waits_when_too_few_logins_are_free(self):
        logins = ["A", "B", "C", "L"]
        first = journey.claim_logins(["A", "B"], logins)[1]
        waits = []

        def sleep(seconds):
            waits.append(seconds)
            for handle in first:
                handle.close()

        with redirect_stdout(StringIO()):
            mapping, handles = journey.claim_logins(["A", "B"], logins, sleep=sleep)
        self.hold(*handles)
        self.assertEqual(len(waits), 1)
        self.assertEqual(mapping, {"A": "A", "B": "B"})

    def test_a_partial_claim_is_given_back_before_waiting(self):
        logins = ["A", "B", "C"]
        self.hold(*journey.claim_logins(["A", "B"], logins)[1])

        def sleep(seconds):
            free = journey.try_lock(journey.ACTOR_LOCK % "C")
            self.assertIsNotNone(free)
            free.close()
            raise StopIteration

        with redirect_stdout(StringIO()), self.assertRaises(StopIteration):
            journey.claim_logins(["A", "B"], logins, sleep=sleep)

    def test_the_link_number_is_not_a_spare_login(self):
        def raise_stop(seconds):
            raise StopIteration

        mapping, handles = journey.claim_logins(["L"], ["A", "L"])
        self.hold(*handles)
        self.assertEqual(mapping, {"L": "L"})
        self.hold(*journey.claim_logins(["A"], ["A", "L"])[1])
        with redirect_stdout(StringIO()):
            with self.assertRaises(StopIteration):
                journey.claim_logins(["A"], ["A", "L"], sleep=raise_stop)

    def test_a_remapped_actor_gets_the_logins_credentials(self):
        def row(actor, phone):
            return {"actor": actor, "phone": phone, "email": "", "code": "", "name": ""}

        accounts = {"A": row("A", "1"), "C": row("C", "3"), "L": row("L", "9")}
        result = journey.logins_for(accounts, {"A": "C"})
        self.assertEqual(sorted(result), ["A", "L"])
        env = journey.actor_environment(result, "sms", "R")
        self.assertEqual(env["MONACO_QA_A_PHONE"], "3")
        self.assertNotIn("MONACO_QA_C_PHONE", env)


class JourneyPsql(unittest.TestCase):
    """apps/mobile/qa/journeys/psql.sh, run with fake psql and docker on PATH."""

    SCRIPT = journey.QA / "psql.sh"

    def run_with(self, tools):
        folder = tempfile.TemporaryDirectory()
        self.addCleanup(folder.cleanup)
        bin_dir = Path(folder.name)
        log = bin_dir / "calls"
        for name, body in tools.items():
            tool = bin_dir / name
            tool.write_text("#!/bin/bash\n" + body.replace("LOG", str(log)) + "\n")
            tool.chmod(0o755)
        # Only the fakes: a runner's real /usr/bin/psql must not answer for them.
        env = {"PATH": str(bin_dir),
               "DATABASE_URL": "postgres://monaco:monaco@localhost:54322/monaco?sslmode=disable"}
        done = subprocess.run(["/bin/bash", str(self.SCRIPT), "-tA"], input="SELECT 1\n", env=env,
                              stdout=subprocess.PIPE, stderr=subprocess.PIPE, universal_newlines=True)
        return done, log.read_text() if log.exists() else ""

    def test_host_psql_is_used_when_installed(self):
        done, calls = self.run_with({
            "psql": 'echo "psql $*" >> LOG; /bin/cat',
            "docker": 'echo "docker $*" >> LOG',
        })
        self.assertEqual(done.returncode, 0)
        self.assertEqual(done.stdout, "SELECT 1\n")
        self.assertEqual(calls, "psql postgres://monaco:monaco@localhost:54322/monaco?sslmode=disable -tA\n")

    def test_without_host_psql_it_runs_inside_the_compose_container(self):
        done, calls = self.run_with({"docker": """echo "docker $*" >> LOG
if [[ $1 == inspect ]]; then echo true; else /bin/cat; fi"""})
        self.assertEqual(done.returncode, 0)
        self.assertEqual(done.stdout, "SELECT 1\n")
        self.assertIn("docker exec -i monaco-postgres sh -c", calls)
        self.assertTrue(calls.rstrip().endswith("monaco -tA"), calls)

    def test_neither_says_psql_is_missing_not_that_the_database_is_down(self):
        done, calls = self.run_with({"docker": 'echo "docker $*" >> LOG; echo false'})
        self.assertEqual(done.returncode, 127)
        self.assertEqual(done.stderr, "psql not found (install it, or start Compose postgres)\n")
        self.assertNotIn("docker exec", calls)


    def test_health_check_uses_the_configured_base_url(self):
        calls = []

        class Response:
            status = 200

            def close(self):
                return None

        def opener(url, timeout):
            calls.append((url, timeout))
            return Response()

        self.assertTrue(journey.backend_is_running("http://api.example/", opener))
        self.assertEqual(calls, [("http://api.example/healthz", 2)])

    def test_health_check_treats_a_connection_error_as_not_running(self):
        def opener(url, timeout):
            raise OSError("offline")

        self.assertFalse(journey.backend_is_running("http://api.example", opener))


class PruneQACabals(unittest.TestCase):
    """Every run prunes; apps/mobile/qa/journeys/prune.sh runs here with a fake psql on PATH."""

    SCRIPT = journey.QA / "prune.sh"

    def run_script(self, url, psql_output="", lock_dir=None):
        folder = tempfile.TemporaryDirectory()
        self.addCleanup(folder.cleanup)
        bin_dir = Path(folder.name)
        log = bin_dir / "calls"
        tool = bin_dir / "psql"
        tool.write_text("#!/bin/bash\necho \"psql $*\" >> %s\n/bin/cat >> %s\nprintf '%%s' '%s'\n" % (log, log, psql_output))
        tool.chmod(0o755)
        env = {"PATH": "%s:/usr/bin:/bin" % bin_dir, "DATABASE_URL": url,
               "JOURNEY_RUN_STARTED": "2026-10-30T12:00:00Z", "MONACO_QA_LOCK_DIR": lock_dir or folder.name}
        done = subprocess.run(["/bin/bash", str(self.SCRIPT)], env=env, stdout=subprocess.PIPE,
                              stderr=subprocess.PIPE, universal_newlines=True)
        return done, log.read_text() if log.exists() else ""

    def test_every_run_prunes_after_taking_its_run_lock(self):
        events = []
        args = types.SimpleNamespace(timeout=5, channel="sms")
        fake = types.SimpleNamespace(id="auth/sign-in", version=1, known={}, scenarios=["S1"])
        with tempfile.TemporaryDirectory() as locks, tempfile.TemporaryDirectory() as out, \
                unittest.mock.patch.dict(os.environ, {"MONACO_QA_LOCK_DIR": locks}), \
                unittest.mock.patch.object(journey, "OUT", Path(out)), \
                unittest.mock.patch.object(journey, "new_run_id", lambda: "LIVE22"), \
                unittest.mock.patch.object(journey, "prune_qa_cabals", lambda started: events.append(
                    "locked" if journey.try_lock(journey.run_lock_path("LIVE22")) is None else "unlocked")), \
                unittest.mock.patch.object(journey, "run_session", lambda *a, **k: ([], 0, None)), \
                unittest.mock.patch.object(journey, "run_truth", lambda *a, **k: "ok"), \
                unittest.mock.patch.object(journey, "record", lambda *a, **k: None), \
                redirect_stdout(StringIO()):
            journey.run_once(fake, {}, args, "sim", {}, "b", "r", ["S1"], "http://x")
            journey.run_once(fake, {}, args, "sim", {}, "b", "r", ["S1"], "http://x")
        self.assertEqual(events, ["locked", "locked"])

    def test_a_held_run_lock_is_live_and_a_released_one_is_not(self):
        with tempfile.TemporaryDirectory() as locks:
            with unittest.mock.patch.dict(os.environ, {"MONACO_QA_LOCK_DIR": locks}):
                with journey.run_lock("HELD22"):
                    Path(journey.run_lock_path("DEAD22")).write_text("")
                    done, calls = self.run_script("postgres://monaco:monaco@localhost:54322/monaco", lock_dir=locks)
                    self.assertIn("-v live={HELD22}", calls)
                done, calls = self.run_script("postgres://monaco:monaco@localhost:54322/monaco", lock_dir=locks)
                self.assertIn("-v live={}", calls)
        self.assertIn("right(c.name, 6) = ANY", calls)
        self.assertIn("created_at < :'started'::timestamptz - interval '1 hour'", calls)

    def test_a_failed_prune_warns_and_does_not_raise(self):
        def runner(args, **kwargs):
            return types.SimpleNamespace(returncode=3)

        err = StringIO()
        with contextlib.redirect_stderr(err):
            journey.prune_qa_cabals(journey.datetime.datetime.now(journey.datetime.timezone.utc), runner=runner)
        self.assertIn("prune of old QA cabals failed (exit 3)", err.getvalue())

    def test_a_remote_database_is_refused_without_a_psql_call(self):
        for url in ("postgres://u:p@db.example.com:5432/monaco", "postgres://u:p@localhost.evil.com/monaco"):
            done, calls = self.run_script(url)
            self.assertEqual(done.returncode, 0)
            self.assertIn("refused", done.stdout)
            self.assertEqual(calls, "")

    def test_a_local_database_gets_one_transaction_that_skips_funded_cabals(self):
        done, calls = self.run_script("postgres://monaco:monaco@[::1]:54322/monaco",
                                      "skipped|id1|QA rich\nclosed|id2|QA old\nleft|id3|QA other\n")
        self.assertEqual(done.returncode, 0, done.stderr)
        self.assertEqual(calls.count("psql "), 1)
        for text in ("BEGIN;", "COMMIT;", "fund_transfers", "cabal_positions", "p.units > 0",
                     "SET status = 'banned'", "DELETE FROM cabal_members", "LIKE 'QA %'", "interval '1 hour'",
                     "started=2026-10-30T12:00:00Z", "DELETE FROM rate_limit_buckets", ":actor:user:"):
            self.assertIn(text, calls)
        self.assertNotIn("DELETE FROM cabals", calls)
        self.assertIn("prune: skipped cabal id1 (QA rich)", done.stdout)
        self.assertIn("prune: pruned 2 cabals, skipped 1 funded", done.stdout)


class Runner(Tree):
    def test_the_truth_check_receives_the_actor_environment(self):
        loaded = journey.load_journeys()["auth/sign-in"]
        self.write("qa/auth/sign-in.truth.sh", "#!/usr/bin/env bash\n")
        seen = []
        saved_sh = journey.sh

        def stub(args, **kwargs):
            seen.append(kwargs["env"])
            return type("Result", (), {"returncode": 0})()

        journey.sh = stub
        try:
            self.assertEqual(journey.run_truth(
                loaded, journey.load_accounts(environ={}), "sms", "RUN123", journey.ROOT / "run" / "handoff.json"), "ok")
        finally:
            journey.sh = saved_sh
        self.assertEqual(seen[0]["MONACO_QA_CHANNEL"], "sms")
        self.assertEqual(seen[0]["MONACO_QA_RUN"], "RUN123")
        self.assertEqual(seen[0]["MONACO_QA_HANDOFF"], str(journey.ROOT / "run" / "handoff.json"))


class Simulators(Tree):
    def setUp(self):
        super().setUp()
        self.saved_sh = journey.sh
        self.saved_lane_name = journey.lane_name
        journey.lane_name = lambda: None
        self.loaded = journey.load_journeys()["auth/sign-in"]
        self.devices = {
            "com.apple.CoreSimulator.SimRuntime.iOS-26-5": [{
                "udid": "gold",
                "name": "Monaco Gold",
                "isAvailable": True,
                "deviceTypeIdentifier": "phone",
            }],
        }

    def tearDown(self):
        journey.sh = self.saved_sh
        journey.lane_name = self.saved_lane_name
        super().tearDown()

    def result(self, stdout="", returncode=0):
        return type("Result", (), {"stdout": stdout, "returncode": returncode})()

    def stub_for(self, calls, created="journey"):
        def stub(args, **kwargs):
            calls.append(args)
            if args == ["scripts/gold-sim-udid.sh"]:
                return self.result("gold\n")
            if args == ["xcrun", "simctl", "list", "devices", "--json"]:
                return self.result(json.dumps({"devices": self.devices}))
            if args[:3] == ["xcrun", "simctl", "create"]:
                return self.result(created + "\n")
            if args[:2] == ["git", "rev-parse"]:
                return self.result(self.tmp.name + "\n")
            if args[0].endswith("simslim-ensure.sh"):
                return self.result()
            self.fail("unexpected command: %r" % (args,))
        return stub

    def test_one_dedicated_simulator_is_built_like_gold_whatever_the_actors(self):
        calls = []
        journey.sh = self.stub_for(calls)
        self.assertEqual(journey.resolve_simulator(), "journey")
        self.assertEqual([c for c in calls if c[:3] == ["xcrun", "simctl", "create"]], [[
            "xcrun", "simctl", "create", "Monaco Journeys", "phone",
            "com.apple.CoreSimulator.SimRuntime.iOS-26-5"]])

    def test_a_lane_names_its_simulator_after_itself(self):
        calls = []
        self.devices["other"] = [{"udid": "primary", "name": "Monaco Journeys", "isAvailable": True}]
        journey.sh = self.stub_for(calls, "lane")
        journey.lane_name = lambda: "agent-7"
        self.assertEqual(journey.resolve_simulator(), "lane")
        self.assertIn(["xcrun", "simctl", "create", "Monaco Journeys agent-7", "phone",
                       "com.apple.CoreSimulator.SimRuntime.iOS-26-5"], calls)
        registry = Path(self.tmp.name, "monaco-lane-sims.tsv").read_text()
        self.assertEqual(registry, "lane\tagent-7\tMonaco Journeys agent-7\n")

    def test_a_new_simulator_is_slimmed_and_an_existing_one_is_checked(self):
        calls = []
        journey.sh = self.stub_for(calls)
        journey.resolve_simulator()
        ensure = [c[1:] for c in calls if c[0].endswith("simslim-ensure.sh")]
        self.assertEqual(ensure, [["create", "journey"]])
        self.devices["other"] = [{"udid": "have", "name": "Monaco Journeys", "isAvailable": True}]
        del calls[:]
        self.assertEqual(journey.resolve_simulator(), "have")
        ensure = [c[1:] for c in calls if c[0].endswith("simslim-ensure.sh")]
        self.assertEqual(ensure, [["check", "have"]])

    def ensure_envs(self, **kwargs):
        envs = []
        stub = self.stub_for([])
        def capture(args, **kw):
            if args[0].endswith("simslim-ensure.sh"):
                envs.append(kw.get("env"))
            return stub(args, **kw)
        journey.sh = capture
        self.devices["other"] = [{"udid": "have", "name": "Monaco Journeys", "isAvailable": True}]
        journey.resolve_simulator(**kwargs)
        return envs

    def test_a_journey_profile_reaches_simslim_ensure_as_simslim_profile(self):
        envs = self.ensure_envs(profile="photos")
        self.assertEqual(len(envs), 1)
        self.assertEqual(envs[0]["SIMSLIM_PROFILE"], str(journey.ROOT / "ci" / "profiles" / "photos.json"))
        self.assertTrue(envs[0]["SIMSLIM_PROFILE"].endswith("ci/profiles/photos.json"))

    def test_simslim_takes_the_sim_slot_before_it_boots_the_simulator(self):
        order = []
        with unittest.mock.patch.object(journey, "take_sim_slot", lambda udid: order.append(("slot", udid))), \
                unittest.mock.patch.object(journey, "sh", lambda args, **kw: order.append(("sh", os.path.basename(args[0])))):
            journey.simslim_ensure("create", "U1")
        self.assertEqual(order, [("slot", "U1"), ("sh", "simslim-ensure.sh")])

    def test_a_journey_without_a_profile_leaves_the_environment_alone(self):
        self.assertEqual(self.ensure_envs(), [None])

    def test_a_created_simulator_is_slimmed_with_the_profile_too(self):
        envs = []
        stub = self.stub_for([])
        def capture(args, **kw):
            if args[0].endswith("simslim-ensure.sh"):
                envs.append(kw.get("env"))
            return stub(args, **kw)
        journey.sh = capture
        journey.resolve_simulator(profile="photos")
        self.assertTrue(envs[0]["SIMSLIM_PROFILE"].endswith("ci/profiles/photos.json"))

    def test_an_explicit_simulator_is_kept_and_not_slimmed(self):
        journey.sh = lambda args, **kwargs: self.fail("unexpected command: %r" % (args,))
        self.assertEqual(journey.resolve_simulator("mine"), "mine")

    def test_lane_name_is_the_linked_worktree_directory(self):
        journey.lane_name = self.saved_lane_name
        root = Path(self.tmp.name)
        git = ["git", "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"]
        subprocess.run(git + ["init", "-q"], cwd=str(root), check=True)
        subprocess.run(git + ["commit", "-q", "--allow-empty", "--no-verify", "-m", "x"], cwd=str(root), check=True)
        self.assertIsNone(journey.lane_name())
        subprocess.run(git + ["worktree", "add", "-q", str(root / "wt" / "agent-7")], cwd=str(root), check=True)
        journey.ROOT = root / "wt" / "agent-7"
        self.assertEqual(journey.lane_name(), "agent-7")

    def test_a_conflicting_simulator_environment_stops_the_run(self):
        def stub(args, **kwargs):
            if args == ["xcrun", "simctl", "boot", "journey-a"]:
                return self.result()
            if args == ["xcrun", "simctl", "bootstatus", "journey-a", "-b"]:
                return self.result()
            if args == ["xcrun", "simctl", "getenv", "journey-a", "MONACO_API_BASE_URL"]:
                return self.result("http://127.0.0.1:8082\n")
            self.fail("unexpected command: %r" % (args,))

        journey.sh = stub
        with self.assertRaisesRegex(journey.JourneyError, "simulator journey-a has MONACO_API_BASE_URL=http://127.0.0.1:8082"):
            journey.check_simulator_api_environment("journey-a", "http://127.0.0.1:8080")

    def test_a_dedicated_simulator_is_reset_without_fresh_and_an_explicit_one_is_not(self):
        calls = []
        journey.sh = lambda args, **kwargs: calls.append(args) or self.result()
        journey.reset_journey_simulator("chosen", True, False)
        self.assertEqual(calls, [])
        journey.reset_journey_simulator("journey", False, False)
        self.assertEqual(calls, [
            ["xcrun", "simctl", "boot", "journey"],
            ["xcrun", "simctl", "bootstatus", "journey", "-b"],
            ["xcrun", "simctl", "uninstall", "journey", "xyz.monacolabs.app"],
        ])

    def test_fresh_resets_an_explicit_simulator(self):
        calls = []
        journey.sh = lambda args, **kwargs: calls.append(args) or self.result()
        journey.reset_journey_simulator("gold", True, True)
        self.assertEqual(calls, [
            ["xcrun", "simctl", "boot", "gold"],
            ["xcrun", "simctl", "bootstatus", "gold", "-b"],
            ["xcrun", "simctl", "uninstall", "gold", "xyz.monacolabs.app"],
        ])


class Shutdown(unittest.TestCase):
    def setUp(self):
        self.saved_sh = journey.sh
        self.calls = []
        self.booted_before = []

        def stub(args, **kwargs):
            self.calls.append(args)
            if args == ["xcrun", "simctl", "list", "devices", "--json"]:
                items = [{"udid": udid, "state": "Booted"} for udid in self.booted_before]
                return type("Result", (), {"stdout": json.dumps({"devices": {"runtime": items}}), "returncode": 0})()
            return type("Result", (), {"stdout": "", "returncode": 0})()

        journey.sh = stub

    def tearDown(self):
        journey.sh = self.saved_sh

    def shutdowns(self):
        return [call[3] for call in self.calls if call[:3] == ["xcrun", "simctl", "shutdown"]]

    def test_the_sims_a_run_booted_are_shut_down_after_a_pass(self):
        with journey.simulator_shutdown():
            journey.boot_simulator("lane")
            journey.boot_simulator("actor-b")
        self.assertEqual(self.shutdowns(), ["lane", "actor-b"])

    def test_the_sims_are_shut_down_after_a_fail_and_after_an_exception(self):
        for error in (journey.JourneyError("fail"), KeyboardInterrupt()):
            del self.calls[:]
            with self.assertRaises(type(error)):
                with journey.simulator_shutdown():
                    journey.boot_simulator("lane")
                    raise error
            self.assertEqual(self.shutdowns(), ["lane"])

    def test_each_booted_sim_holds_a_sim_slot_from_before_its_boot_until_after_its_shutdown(self):
        journey.take_sim_slot = lambda udid: self.calls.append(["slot", "take", udid])
        journey.release_sim_slot = lambda udid: self.calls.append(["slot", "release", udid])
        saved = journey.SIM_SLOTS
        journey.SIM_SLOTS = {"lane": object(), "actor-b": object()}
        try:
            with journey.simulator_shutdown():
                journey.boot_simulator("lane")
                journey.boot_simulator("actor-b")
        finally:
            journey.SIM_SLOTS = saved
        steps = [tuple(call[:3]) if call[0] == "slot" else (call[2], call[3]) for call in self.calls
                 if call[0] == "slot" or call[2] in ("boot", "shutdown")]
        self.assertEqual(steps, [
            ("slot", "take", "lane"), ("boot", "lane"),
            ("slot", "take", "actor-b"), ("boot", "actor-b"),
            ("shutdown", "lane"), ("shutdown", "actor-b"),
            ("slot", "release", "lane"), ("slot", "release", "actor-b"),
        ])

    def test_a_failed_boot_gives_the_slot_back(self):
        journey.take_sim_slot = lambda udid: self.calls.append(["slot", "take", udid])
        journey.release_sim_slot = lambda udid: self.calls.append(["slot", "release", udid])

        def failing(args, **kwargs):
            if args[:3] == ["xcrun", "simctl", "bootstatus"]:
                raise subprocess.CalledProcessError(1, args)
            return type("Result", (), {"stdout": "", "returncode": 0})()

        journey.sh = failing
        with self.assertRaises(subprocess.CalledProcessError):
            journey.boot_simulator("lane")
        self.assertEqual(self.calls, [["slot", "take", "lane"], ["slot", "release", "lane"]])

    def test_a_sim_that_was_already_booted_is_left_booted(self):
        self.booted_before = ["mine"]
        with journey.simulator_shutdown():
            journey.boot_simulator("mine")
            journey.boot_simulator("new")
        self.assertEqual(self.shutdowns(), ["new"])

    def test_keep_sims_skips_the_shutdown(self):
        with journey.simulator_shutdown(keep=True):
            journey.boot_simulator("lane")
        self.assertEqual(self.shutdowns(), [])

    def test_sigterm_shuts_the_sims_down(self):
        with self.assertRaises(KeyboardInterrupt):
            with journey.simulator_shutdown():
                journey.boot_simulator("lane")
                os.kill(os.getpid(), journey.signal.SIGTERM)
                time.sleep(1)
        self.assertEqual(self.shutdowns(), ["lane"])

    def test_the_keep_sims_flag_is_parsed(self):
        seen = []
        saved = journey.cmd_run
        journey.cmd_run = lambda args: seen.append(args.keep_sims) or 0
        try:
            journey.main(["run", "auth/sign-in", "--keep-sims"])
            journey.main(["run", "auth/sign-in"])
        finally:
            journey.cmd_run = saved
        self.assertEqual(seen, [True, False])


LOG = """Test Case '-[MonacoUITests.SignInJourneyUITests testS1SignIn]' started.
JOURNEYSTEP\tbegin\tauth/sign-in@2\tP1\tlaunch
JOURNEYSTEP\tend\tauth/sign-in@2\tP1\t1500
JOURNEYSTEP\tbegin\tauth/sign-in@2\tS1.1\tchoose
JOURNEYSTEP\tend\tauth/sign-in@2\tS1.1\t500
Test Case '-[MonacoUITests.SignInJourneyUITests testS1SignIn]' passed (2.250 seconds).
Test Case '-[MonacoUITests.SignInJourneyUITests testS2Relaunch]' started.
JOURNEYSTEP\tbegin\tauth/sign-in@2\tP1\tlaunch
JOURNEYSTEP\tbegin\tauth/sign-in@2\tS3.1\topen Profile
SignInJourney.swift:127: error: XCTAssertTrue failed - S3.1: no Sign out button
Test Case '-[MonacoUITests.SignInJourneyUITests testS2Relaunch]' failed (40.000 seconds).
"""


class SimSlots(unittest.TestCase):
    """take_sim_slot and release_sim_slot against the real scripts/qa/xcode-lock.sh, in a scratch lock dir."""

    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.addCleanup(self.dir.cleanup)
        patched = unittest.mock.patch.dict(os.environ, {
            "MONACO_SIM_LOCK_DIR": self.dir.name + "/sim.lock", "MONACO_SIM_SLOTS": "2", "MONACO_LOCK_POLL": "0.1"})
        patched.start()
        self.addCleanup(patched.stop)
        self.addCleanup(lambda: [REAL_RELEASE_SIM_SLOT(udid) for udid in list(journey.SIM_SLOTS)])

    def slot_dirs(self):
        return sorted(name for name in os.listdir(self.dir.name) if name.startswith("sim.lock") and "queue" not in name)

    def test_each_simulator_holds_one_slot_until_it_is_released(self):
        REAL_TAKE_SIM_SLOT("a")
        REAL_TAKE_SIM_SLOT("b")
        REAL_TAKE_SIM_SLOT("a")
        self.assertEqual(self.slot_dirs(), ["sim.lock", "sim.lock.2"])
        REAL_RELEASE_SIM_SLOT("a")
        self.assertEqual(self.slot_dirs(), ["sim.lock.2"])
        REAL_RELEASE_SIM_SLOT("a")
        REAL_RELEASE_SIM_SLOT("b")
        self.assertEqual(self.slot_dirs(), [])

    def test_a_holder_that_never_says_held_is_an_error(self):
        class Dead:
            returncode = 75
            stdout = StringIO("")

            def wait(self):
                return 75

        with unittest.mock.patch.object(journey.subprocess, "Popen", lambda *a, **k: Dead()):
            with self.assertRaisesRegex(journey.JourneyError, "could not take a simulator slot for a"):
                REAL_TAKE_SIM_SLOT("a")
        self.assertEqual(journey.SIM_SLOTS, {})


class Logs(unittest.TestCase):
    def test_each_test_gets_its_verdict_time_and_slice(self):
        tests = journey.split_by_test(LOG)
        self.assertEqual(tests["testS1SignIn"][:2], ("passed", 2.25))
        self.assertEqual(tests["testS2Relaunch"][:2], ("failed", 40.0))
        self.assertNotIn("S3.1", tests["testS1SignIn"][2])

    def test_finished_outermost_steps_are_summed_and_the_innermost_open_step_failed(self):
        tests = journey.split_by_test(LOG)
        self.assertEqual(journey.parse_steps(tests["testS1SignIn"][2]), (2000, ""))
        self.assertEqual(journey.parse_steps(tests["testS2Relaunch"][2]), (0, "S3.1"))

    def test_nested_finished_steps_record_depth_without_double_counting(self):
        output = "\n".join([
            "JOURNEYSTEP\tbegin\tauth/sign-in@2\tP1\tlaunch",
            "JOURNEYSTEP\tbegin\tauth/sign-in@2\tS1.1\tchoose",
            "JOURNEYSTEP\tend\tauth/sign-in@2\tS1.1\t500",
            "JOURNEYSTEP\tend\tauth/sign-in@2\tP1\t1500",
        ])
        self.assertEqual(journey.parse_steps(output, include_timings=True), (1500, "", [
            ("S1.1", 500, 1), ("P1", 1500, 0),
        ]))


class Report(unittest.TestCase):
    def row(self, **values):
        base = dict.fromkeys(journey.COLUMNS, "")
        base.update({"journey": "auth/sign-in", "driver": "xcuitest", "build": "abc", "expected": "PASS", "truth": "none"})
        base.update(values)
        return base

    def test_speed_comes_from_clean_passes_and_catch_rate_from_seeded_builds(self):
        rows = [
            self.row(scenario="*", result="PASS", wall_s="100", steps_ms="60000"),
            self.row(scenario="*", result="PASS", wall_s="120", steps_ms="80000"),
            self.row(scenario="*", result="FAIL", wall_s="300"),
            self.row(scenario="*", result="ERROR", wall_s="0"),
            self.row(scenario="*", result="PASS", wall_s="1", build="abc+mutant:x"),
            self.row(scenario="S1", result="FAIL", expected="FAIL", build="abc+mutant:x"),
            self.row(scenario="S2", result="FAIL", expected="FAIL", build="abc+mutant:x"),
            self.row(scenario="S1", result="PASS", expected="FAIL", build="abc+mutant:y"),
        ]
        item = journey.summarize(rows)[0]
        self.assertEqual(item["runs"], 3)
        self.assertEqual(item["errors"], 1)
        self.assertAlmostEqual(item["flake"], 1 / 3.0)
        self.assertEqual(item["wall_s"], 110)
        self.assertEqual(item["steps_s"], 70)
        self.assertEqual((item["caught"], item["seeded"], item["false_passes"]), (1, 2, 1))

    def test_a_pass_the_ground_truth_denies_is_a_false_pass(self):
        item = journey.summarize([self.row(scenario="*", result="PASS", wall_s="10", truth="fail")])[0]
        self.assertEqual(item["false_passes"], 1)


class Output(Tree):
    """Points results, builds and run folders at the throwaway tree."""

    def setUp(self):
        super().setUp()
        self.saved_out = (journey.OUT, journey.DERIVED, journey.RESULTS)
        self.saved_devices = journey.simulator_devices
        journey.simulator_devices = lambda: {}
        journey.OUT = journey.ROOT / "out"
        journey.DERIVED = journey.OUT / "derived"
        journey.RESULTS = journey.OUT / "results.tsv"

    def tearDown(self):
        journey.OUT, journey.DERIVED, journey.RESULTS = self.saved_out
        journey.simulator_devices = self.saved_devices
        super().tearDown()


class XctestrunCommand(Output):
    """A test of an existing build runs from its xctestrun without a build slot; a build keeps the slot."""

    def command(self, *extra):
        saved, journey.cache_args = journey.cache_args, lambda: []
        try:
            return journey.xcodebuild("sim", *extra)
        finally:
            journey.cache_args = saved

    def test_a_test_without_an_xctestrun_keeps_the_project_form_and_its_slot(self):
        args = self.command("-only-testing:MonacoUITests/X", "test-without-building")
        self.assertEqual(args[:3], ["scripts/qa/xcode-lock.sh", "xcode", "xcodebuild"])
        self.assertIn("-project", args)

    def test_a_test_runs_from_the_newest_app_xctestrun_with_no_slot(self):
        products = journey.DERIVED / "Build" / "Products"
        old = self.write(os.path.relpath(str(products / "Monaco_Monaco_iphonesimulator26.5-arm64.xctestrun"),
                                         str(journey.ROOT)), "")
        self.write(os.path.relpath(str(products / "Monaco_MonacoTests_iphonesimulator27.1-arm64.xctestrun"),
                                   str(journey.ROOT)), "")
        new = self.write(os.path.relpath(str(products / "Monaco_Monaco_iphonesimulator27.1-arm64.xctestrun"),
                                         str(journey.ROOT)), "")
        os.utime(str(old), (1, 1))
        args = self.command("-only-testing:MonacoUITests/X", "-collect-test-diagnostics", "never",
                            "test-without-building")
        self.assertEqual(args, ["xcodebuild", "test-without-building", "-xctestrun", str(new),
                                "-destination", "platform=iOS Simulator,id=sim",
                                "-only-testing:MonacoUITests/X", "-collect-test-diagnostics", "never"])

    def test_a_build_keeps_its_slot_when_an_xctestrun_exists(self):
        products = journey.DERIVED / "Build" / "Products"
        self.write(os.path.relpath(str(products / "Monaco_Monaco_iphonesimulator27.1-arm64.xctestrun"),
                                   str(journey.ROOT)), "")
        args = self.command("build-for-testing")
        self.assertEqual(args[:3], ["scripts/qa/xcode-lock.sh", "xcode", "xcodebuild"])
        self.assertEqual(args[-1], "build-for-testing")


class SlotRelease(unittest.TestCase):
    """Two multi-journey calls that share the simulator slots and the logins never wait on each other."""

    def stream(self, names, logins, slots, barrier, result):
        locks = {}

        class Holder:
            def __init__(self):
                self.stdin = self.stdout = self
                self.released = False

            def close(self):
                if not self.released:
                    self.released = True
                    slots.release()

            def wait(self):
                return 0

        def run_journey(args, url, journey_id, builder):
            login = logins[journey_id[-1]]
            login.acquire()
            try:
                if not slots.acquire(timeout=3):
                    return 9
                journey.SIM_SLOTS["sim-" + journey_id] = Holder()
                if journey_id.endswith("1"):
                    barrier.wait(timeout=5)
            finally:
                login.release()
            return 0

        journeys = {name: unittest.mock.Mock(id=name, funds={}, requires=[]) for name in names}
        with unittest.mock.patch.object(journey, "run_journey", run_journey), \
                unittest.mock.patch.object(journey, "release_sim_slot", REAL_RELEASE_SIM_SLOT), \
                unittest.mock.patch.object(journey, "load_journeys", lambda: journeys), \
                unittest.mock.patch.object(journey, "simulator_devices", lambda: {}), \
                redirect_stdout(StringIO()):
            args = unittest.mock.Mock(keep_sims=False)
            result.put(journey.run_all(args, "http://127.0.0.1:8080", lambda sim: None, only=names))

    def test_two_streams_with_crossed_logins_both_finish(self):
        ctx = multiprocessing.get_context("fork")
        slots, barrier, result = ctx.Semaphore(2), ctx.Barrier(2), ctx.Queue()
        logins = {"A": ctx.Lock(), "B": ctx.Lock()}
        streams = [ctx.Process(target=self.stream, args=(names, {"1": logins[a], "2": logins[b]}, slots, barrier, result))
                   for names, a, b in ((["x1", "x2"], "A", "B"), (["y1", "y2"], "B", "A"))]
        for process in streams:
            process.start()
        for process in streams:
            process.join(timeout=20)
        self.assertEqual(sorted(result.get(timeout=1) for _ in streams), [0, 0])


class RemappedRun(Output):
    """A doc-actor-A journey that holds login C must not touch A's simulator or derived data."""

    def accounts(self):
        row = {"actor": "A", "phone": "3", "email": "", "code": "", "name": "", "login": "C"}
        return {"A": row}

    def derived_arg(self):
        saved, journey.cache_args = journey.cache_args, lambda: []
        try:
            args = journey.xcodebuild("sim")
        finally:
            journey.cache_args = saved
        return args[args.index("-derivedDataPath") + 1]

    def test_held_logins_follow_the_remapping(self):
        loaded = journey.load_journeys()["auth/sign-in"]
        self.assertEqual(journey.held_logins(loaded, self.accounts()), {"A": "C"})

    def test_a_remapped_actor_gets_the_held_logins_simulator(self):
        loaded = journey.load_journeys()["auth/sign-in"]
        created = []

        def result(stdout=""):
            return type("R", (), {"stdout": stdout, "returncode": 0})()

        devices = {"rt": [{"udid": "gold", "name": "Monaco Gold", "isAvailable": True,
                           "deviceTypeIdentifier": "phone"},
                          {"udid": "sim-a", "name": "Monaco Journeys A", "isAvailable": True}]}

        def stub(args, **kwargs):
            if args == ["scripts/gold-sim-udid.sh"]:
                return result("gold\n")
            if args[:3] == ["xcrun", "simctl", "create"]:
                created.append(args[3])
                return result("sim-c\n")
            return result()

        logins = journey.held_logins(loaded, self.accounts())
        with unittest.mock.patch.object(journey, "sh", stub), \
                unittest.mock.patch.object(journey, "simulator_devices", lambda: devices), \
                unittest.mock.patch.object(journey, "lane_name", lambda: None), redirect_stdout(StringIO()):
            sim = journey.resolve_simulator("", logins)
        self.assertEqual(sim, "sim-c")
        self.assertEqual(created, ["Monaco Journeys C"])

    def test_a_remapped_run_builds_into_its_own_derived_data(self):
        loaded = journey.load_journeys()["auth/sign-in"]
        with unittest.mock.patch.object(journey, "DERIVED", journey.DERIVED):
            journey.use_derived_data(journey.held_logins(loaded, self.accounts()))
            self.assertEqual(self.derived_arg(), str(journey.OUT / "derived-C"))
            journey.use_derived_data({"A": "A", "B": "B"})
            self.assertEqual(self.derived_arg(), str(journey.OUT / "derived-A-B"))


class RemappedSetup(Output):
    """A doc actor remapped onto another login seeds that login, not its own."""

    def setUp(self):
        super().setUp()
        self.write("qa/accounts.tsv", "# logins\nactor\tname\tphone\temail\tcode\tprivy_user_id\n"
                   "A\tAlfred\t555\ta@b.c\t123456\tdid:privy:aaa\nC\tCayman\t777\tc@b.c\t654321\tdid:privy:ccc\n")

    def accounts(self):
        loaded = journey.load_accounts(environ={})
        return journey.logins_for(loaded, {"A": "C"})

    def test_the_run_accounts_file_gives_the_actor_the_held_logins_row(self):
        path = journey.write_run_accounts(self.accounts())
        rows = {line.split("\t")[0]: line.split("\t") for line in Path(path).read_text().splitlines()}
        header = rows["actor"]
        original = journey.load_accounts(environ={})
        self.assertEqual(rows["A"][header.index("privy_user_id")], original["C"]["privy_user_id"])
        self.assertEqual(rows["A"][header.index("phone")], original["C"]["phone"])
        self.assertEqual(rows["C"][header.index("privy_user_id")], original["C"]["privy_user_id"])

    def test_runs_with_different_logins_write_different_files(self):
        original = journey.load_accounts(environ={})
        self.assertNotEqual(journey.write_run_accounts(self.accounts()), journey.write_run_accounts(original))
        self.assertEqual(journey.write_run_accounts(self.accounts()), journey.write_run_accounts(self.accounts()))

    def test_a_setup_script_reads_the_held_logins_account(self):
        path = journey.write_run_accounts(self.accounts())
        script = ('source scripts/qa/seed.sh; _qa_account A privy_user_id; '
                  'apps/mobile/qa/journeys/privy-user-id.sh A')
        env = dict(os.environ, QA_ACCOUNTS_FILE=path)
        done = subprocess.run(["bash", "-c", script], cwd=str(Path(__file__).resolve().parents[2]),
                              env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, universal_newlines=True)
        want = journey.load_accounts(environ={})["C"]["privy_user_id"]
        self.assertEqual(done.stdout.split(), [want, want], done.stderr)

    def test_prepare_logins_runs_the_script_for_each_actor_with_the_run_accounts(self):
        loaded = journey.load_journeys()["auth/sign-in"]
        seen = []

        def stub(args, **kwargs):
            seen.append((args, kwargs["env"]))
            return type("R", (), {"stdout": "ready\n", "returncode": 0})()

        with unittest.mock.patch.object(journey, "sh", stub), redirect_stdout(StringIO()):
            journey.prepare_logins(loaded, self.accounts(), "http://127.0.0.1:8180")
        self.assertEqual([args for args, _ in seen], [["scripts/qa/ready-login.sh", "A"]])
        env = seen[0][1]
        self.assertEqual(env["MONACO_API_BASE_URL"], "http://127.0.0.1:8180")
        self.assertIn("accounts-", env["QA_ACCOUNTS_FILE"])

    def test_a_failed_preparation_stops_the_run(self):
        loaded = journey.load_journeys()["auth/sign-in"]
        with unittest.mock.patch.object(journey, "sh", lambda *a, **k: type("R", (), {"stdout": "", "returncode": 1})()), \
                redirect_stdout(StringIO()):
            with self.assertRaisesRegex(journey.JourneyError, "could not prepare the login for actor A"):
                journey.prepare_logins(loaded, self.accounts(), "http://127.0.0.1:8080")

    @staticmethod
    def refuse_boot(*args, **kwargs):
        raise RuntimeError("stop")

    def test_each_slot_writes_its_own_backend_log(self):
        logs = []
        with tempfile.TemporaryDirectory() as out, unittest.mock.patch.object(journey, "OUT", Path(out)), \
                unittest.mock.patch.object(journey, "apply_event_streams", lambda log, url=None: logs.append(log.name)), \
                unittest.mock.patch.object(journey.subprocess, "Popen", self.refuse_boot), \
                redirect_stdout(StringIO()):
            for slot in (0, 1):
                with self.assertRaises(RuntimeError):
                    journey.start_backend("http://127.0.0.1:8080", slot=slot)
        self.assertEqual(logs, ["backend-slot0.log", "backend-slot1.log"])


class ReadyLogin(unittest.TestCase):
    """scripts/qa/ready-login.sh with qa_api, qa_sql and the account lookup stubbed in bash."""

    REPO = Path(__file__).resolve().parents[2]

    def run_script(self, body, row="|CREATED", taken=("qa_cayman",)):
        script = """set -euo pipefail
source scripts/qa/ready-login.sh
qa_api_ready() { :; }
_qa_account() { case "$2" in privy_user_id) echo did:privy:cmu6gsdaa02g00dlierzbikgz;; name) echo Cayman;; esac; }
qa_sql() { cat >/dev/null; echo "$ROW"; }
qa_api() {
  echo "api $1 $2 $3 $4" >> "$CALLS"
  if [[ "$3" == /v1/me/handle ]]; then
    for h in $TAKEN; do
      if [[ "$4" == *"\\"$h\\""* ]]; then
        echo '{"code":"handle_taken"}' >&2
        echo "qa_api $1 $2 $3: HTTP 422" >&2
        return 1
      fi
    done
  fi
}
""" + body
        with tempfile.TemporaryDirectory() as tmp:
            calls = Path(tmp) / "calls"
            env = dict(os.environ, ROW=row, TAKEN=" ".join(taken), CALLS=str(calls))
            done = subprocess.run(["bash", "-c", script], cwd=str(self.REPO), env=env, stdout=subprocess.PIPE,
                                  stderr=subprocess.PIPE, universal_newlines=True)
            return done, calls.read_text().splitlines() if calls.exists() else []

    def test_the_handle_is_qa_name(self):
        done, _ = self.run_script('qa_handle "Link number"; qa_handle Cayman')
        self.assertEqual(done.stdout.split(), ["qa_linknumber", "qa_cayman"], done.stderr)

    def test_the_fallback_handle_ends_in_six_characters_of_the_privy_id(self):
        done, _ = self.run_script("qa_handle_fallback Cayman did:privy:cmu6gsdaa02g00dlierzbikgz")
        self.assertEqual(done.stdout.strip(), "qa_cayman_cmu6gs", done.stderr)

    def test_a_free_handle_is_used_and_the_phone_step_is_skipped(self):
        done, calls = self.run_script("ready_login A", taken=())
        self.assertEqual(done.returncode, 0, done.stderr)
        self.assertEqual(calls, ['api A PUT /v1/me/handle {"handle":"qa_cayman"}',
                                 'api A POST /v1/me/onboarding/skip {"step":"phone"}'])
        self.assertIn("is @qa_cayman, AWAITING_PHONE", done.stdout)

    def test_a_taken_handle_falls_back_to_the_logins_own(self):
        done, calls = self.run_script("ready_login A")
        self.assertEqual(done.returncode, 0, done.stderr)
        self.assertEqual(calls[:2], ['api A PUT /v1/me/handle {"handle":"qa_cayman"}',
                                     'api A PUT /v1/me/handle {"handle":"qa_cayman_cmu6gs"}'])
        self.assertIn("got the handle qa_cayman_cmu6gs", done.stdout)

    def test_another_failure_is_not_retried_with_a_fallback(self):
        done, calls = self.run_script("qa_api() { echo boom >&2; return 1; }; ready_login A")
        self.assertNotEqual(done.returncode, 0)
        self.assertIn("boom", done.stderr)
        self.assertEqual(calls, [])

    def test_a_member_with_a_handle_and_a_state_past_created_is_left_alone(self):
        done, calls = self.run_script("ready_login A", row="qa_cayman_cmu6gs|AWAITING_PHONE")
        self.assertEqual((done.returncode, calls), (0, []), done.stderr)

    def test_a_login_with_no_users_row_waits_for_its_first_sign_in(self):
        done, calls = self.run_script("ready_login A", row="")
        self.assertEqual((done.returncode, calls), (0, []), done.stderr)
        self.assertIn("signs in once first", done.stdout)


class StopBackend(unittest.TestCase):
    """stop_backend touches this run's process group only: the other slot's api and worker share its binaries."""

    def listing(self, *rows):
        return type("R", (), {"stdout": "".join("%5d %5d %s\n" % row for row in rows), "returncode": 0})()

    def stop(self, steps, own=100):
        calls, signals, groups, sleeps = [], [], [], []
        remaining = list(steps)

        def run(args, **kwargs):
            calls.append(args)
            return remaining.pop(0) if len(remaining) > 1 else remaining[0]

        process = type("P", (), {"pid": own, "wait": lambda self, timeout=None: None})()
        journey.stop_backend(process, grace=1, run=run, kill=lambda pid, sig: signals.append((pid, sig)),
                             killpg=lambda pgid, sig: groups.append((pgid, sig)), sleep=sleeps.append)
        return calls, signals, groups, sleeps

    def test_the_group_lists_only_its_own_processes(self):
        run = lambda args, **kwargs: self.listing(  # noqa: E731
            (100, 100, "just run backend"), (101, 100, "/x/bin/api"), (200, 200, "/x/bin/api"))
        self.assertEqual(journey.backend_group(100, run), [(100, "just run backend"), (101, "/x/bin/api")])

    def test_wrappers_are_killed_first_then_api_and_worker_get_sigterm(self):
        api, worker = str(journey.ROOT / "bin" / "api"), str(journey.ROOT / "bin" / "worker")
        both = self.listing((100, 100, "just run backend"), (101, 100, "bash with-dotenv-local.sh"),
                            (102, 100, api), (103, 100, worker), (200, 200, api), (201, 200, worker))
        gone = self.listing((200, 200, api), (201, 200, worker))
        calls, signals, groups, _ = self.stop([both, gone])
        self.assertEqual(signals, [(100, signal.SIGKILL), (101, signal.SIGKILL),
                                   (102, signal.SIGTERM), (103, signal.SIGTERM)])
        self.assertEqual(groups, [(100, signal.SIGKILL)])
        self.assertNotIn(200, [pid for pid, _ in signals])
        self.assertNotIn(201, [pid for pid, _ in signals])
        self.assertTrue(all(args[0] == "ps" for args in calls), calls)

    def test_it_never_runs_just_stop(self):
        calls, _, _, _ = self.stop([self.listing()])
        self.assertEqual([args for args in calls if args[0] == "just"], [])

    def test_a_service_that_ignores_sigterm_is_killed_after_the_grace(self):
        api = str(journey.ROOT / "bin" / "api")
        stuck = self.listing((102, 100, api))
        _, signals, groups, sleeps = self.stop([stuck])
        self.assertEqual(signals, [(102, signal.SIGTERM)])
        self.assertEqual(groups, [(100, signal.SIGKILL)])
        self.assertEqual(len(sleeps), 5)


class BusApply(unittest.TestCase):
    def test_bus_apply_runs_before_the_backend_boots(self):
        order = []

        def stub(args, **kwargs):
            order.append(" ".join(args))
            return type("R", (), {"returncode": 0})()

        def popen(args, **kwargs):
            order.append("BOOT " + " ".join(args))
            raise StopIteration

        with tempfile.TemporaryDirectory() as out, unittest.mock.patch.object(journey, "OUT", Path(out)), \
                unittest.mock.patch.object(journey, "sh", stub), \
                unittest.mock.patch.object(journey.subprocess, "Popen", popen), redirect_stdout(StringIO()):
            with self.assertRaises(StopIteration):
                journey.start_backend("http://127.0.0.1:8080")
        self.assertEqual(order[-2:], ["scripts/with-dotenv-local.sh bin/monacoctl bus apply", "BOOT just run backend"])
        self.assertLess(order.index("docker compose up -d --wait postgres"), len(order) - 2)

    def test_a_failed_bus_apply_stops_the_run_before_boot(self):
        booted = []
        with tempfile.TemporaryDirectory() as out, unittest.mock.patch.object(journey, "OUT", Path(out)), \
                unittest.mock.patch.object(journey, "sh", lambda args, **k: type("R", (), {"returncode": 1})()), \
                unittest.mock.patch.object(journey.subprocess, "Popen", lambda *a, **k: booted.append(a)), \
                redirect_stdout(StringIO()):
            with self.assertRaisesRegex(journey.JourneyError, "failed, see"):
                journey.start_backend("http://127.0.0.1:8080")
        self.assertEqual(booted, [])


def _gone(probe, wait=5.0, every=0.05):
    """Polls probe until the process is gone (ESRCH, or EPERM once macOS hands the pgid to another user)."""
    deadline = time.monotonic() + wait
    while True:
        try:
            probe()
        except (ProcessLookupError, PermissionError):
            return True
        if time.monotonic() >= deadline:
            return False
        time.sleep(every)


class Gone(unittest.TestCase):
    def test_a_group_that_answers_eperm_is_gone(self):
        eperm = PermissionError(1, "Operation not permitted")
        with unittest.mock.patch.object(os, "killpg", side_effect=eperm):
            self.assertTrue(_gone(lambda: os.killpg(12345, 0)))

    def test_a_live_group_is_not_gone(self):
        p = subprocess.Popen(["sleep", "30"], start_new_session=True)
        self.addCleanup(p.wait)
        self.addCleanup(p.kill)
        self.assertFalse(_gone(lambda: os.killpg(p.pid, 0), wait=0.2))


class Budget(Output):
    def test_a_hung_test_call_is_killed_with_its_group_and_its_scenarios_time_out(self):
        pids = journey.ROOT / "pids"
        hang = self.write("hang.sh", "#!/bin/sh\necho $$ > %s\nsleep 60 &\necho $! >> %s\nwait\n" % (pids, pids))
        hang.chmod(0o755)
        args = ["run", "auth/sign-in", "--no-build", "--timeout", "2"]
        with unittest.mock.patch.object(journey, "journey_backend", lambda *a: _yielding("http://127.0.0.1:8080")), \
                unittest.mock.patch.object(journey, "resolve_simulator", lambda override="", logins=None, profile="": "sim"), \
                unittest.mock.patch.object(journey, "check_simulator_api_environment", lambda sim, url: None), \
                unittest.mock.patch.object(journey, "prepare_logins", lambda *a: None), \
                unittest.mock.patch.object(journey, "reset_journey_simulator", lambda *a: None), \
                unittest.mock.patch.object(journey, "build_label", lambda mutant=None: "abc"), \
                unittest.mock.patch.object(journey, "xcodebuild", lambda sim, *extra: [str(hang)]), \
                redirect_stdout(StringIO()) as printed:
            started = time.monotonic()
            code = journey.main(args)
            took = time.monotonic() - started

        self.assertEqual(code, 1)
        self.assertLess(took, 10)
        self.assertIn("auth/sign-in timed out after 2 s in S1 test", printed.getvalue())
        rows = [line.split("\t") for line in journey.RESULTS.read_text().splitlines()[1:]]
        results = {row[journey.COLUMNS.index("scenario")]: row[journey.COLUMNS.index("result")] for row in rows}
        self.assertEqual(results, {"S1": "TIMEOUT", "S2": "SKIP", "*": "TIMEOUT"})
        leader, child = (int(pid) for pid in pids.read_text().split())
        self.assertTrue(_gone(lambda: os.killpg(leader, 0)), "the hung test's process group outlived its kill")
        self.assertTrue(_gone(lambda: os.kill(child, 0)), "the hung test's child outlived its group kill")

    def test_a_timeout_counts_as_a_failed_run_in_the_report(self):
        row = {"journey": "auth/sign-in", "driver": "xcuitest", "build": "abc", "scenario": "*", "result": "TIMEOUT",
               "expected": "PASS", "truth": "none", "wall_s": "300", "steps_ms": ""}
        self.assertEqual(journey.summarize([row])[0]["flake"], 1.0)


KNOWN_BODY = """
### S1 Sign in

| Step | Action |
| --- | --- |
| S1.1 | tap |
| S1.2 | tap |
| S1.3 | tap |
| S1.4 | tap |

## Known failures on staging

- S1.2 to S1.3: the form is a stub. Blocked by #613 and #651.
- S1.4: no chart, see #9 for history. Blocked by #660.

| Step | What fails | Blocked by |
| --- | --- | --- |
| S1.4, S2.1 | No rows | #617 |
| none | Not a step | #1 |

## Not covered

- S1.1: not a known failure. Blocked by #5.
"""


class KnownFailures(Tree):
    def test_bullets_and_table_rows_map_each_step_to_its_tickets(self):
        known = journey.parse_known(KNOWN_BODY, ["S1.1", "S1.2", "S1.3", "S1.4", "S2.1"])
        self.assertEqual(known, {"S1.2": ("#613", "#651"), "S1.3": ("#613", "#651"),
                                 "S1.4": ("#660", "#617"), "S2.1": ("#617",)})

    def test_check_names_a_known_step_the_doc_does_not_have(self):
        self.write("docs/journeys/auth/sign-in.md", DOC + "\n## Known failures on staging\n\n- S1.9: gone. Blocked by #1.\n")
        self.assertTrue(any("S1.9, which is not a step" in problem for problem in self.problems()))


class KnownRuns(Output):
    """A full run whose xcodebuild prints a canned log: S1 fails at `s1_fails_at`, or both scenarios pass."""

    def run_journey(self, s1_fails_at, known):
        self.write("docs/journeys/auth/sign-in.md", DOC + "\n## Known failures on staging\n\n" + known)
        test = "Test Case '-[MonacoUITests.SignInJourneyUITests testJourney]' %s"
        lines = [test % "started.", "JOURNEYSCENARIO\tbegin\tS1"]
        for step in ("S1.1", "S1.2"):
            lines.append("JOURNEYSTEP\tbegin\t0\t%s" % step)
            if step == s1_fails_at:
                break
            lines.append("JOURNEYSTEP\tend\t0\t%s\t10" % step)
        if s1_fails_at:
            lines += [test % "failed (2.000 seconds)."]
        else:
            lines += ["JOURNEYSCENARIO\tend\tS1\t1000", "JOURNEYSCENARIO\tbegin\tS2", "JOURNEYSTEP\tbegin\t0\tS2.1",
                      "JOURNEYSTEP\tend\t0\tS2.1\t10", "JOURNEYSCENARIO\tend\tS2\t1000", test % "passed (2.000 seconds)."]
        log = self.write("canned.log", "\n".join(lines) + "\n")
        fake = self.write("xcodebuild.sh", "#!/bin/sh\ncat %s\n" % log)
        fake.chmod(0o755)
        with unittest.mock.patch.object(journey, "journey_backend", lambda *a: _yielding("http://127.0.0.1:8080")), \
                unittest.mock.patch.object(journey, "resolve_simulator", lambda override="", logins=None, profile="": "sim"), \
                unittest.mock.patch.object(journey, "check_simulator_api_environment", lambda sim, url: None), \
                unittest.mock.patch.object(journey, "prepare_logins", lambda *a: None), \
                unittest.mock.patch.object(journey, "reset_journey_simulator", lambda *a: None), \
                unittest.mock.patch.object(journey, "build_label", lambda mutant=None: "abc"), \
                unittest.mock.patch.object(journey, "xcodebuild", lambda sim, *extra: [str(fake)]), \
                redirect_stdout(StringIO()) as printed:
            code = journey.main(["run", "auth/sign-in", "--no-build"])
        lines = journey.RESULTS.read_text().splitlines()
        rows = [dict(zip(lines[0].split("\t"), line.split("\t"))) for line in lines[1:]]
        return code, {row["scenario"]: (row["result"], row["expected"]) for row in rows}, printed.getvalue()

    def report(self):
        with redirect_stdout(StringIO()) as printed:
            journey.main(["report"])
        return printed.getvalue()

    def test_a_failure_at_a_known_step_is_known_and_the_run_passes(self):
        code, results, printed = self.run_journey("S1.2", "- S1.2: no form. Blocked by #2140.\n")
        self.assertEqual(code, 0)
        self.assertEqual(results, {"S1": ("KNOWN", "FAIL"), "S2": ("SKIP", "PASS"), "*": ("PASS", "PASS")})
        self.assertIn("S1 KNOWN 0.0s at S1.2 (#2140)", printed)
        self.assertIn("S2 SKIP 0.0s after S1 FAIL", printed)
        self.assertIn("| auth/sign-in | %s | - | S1 at S1.2 (#2140) | - | - |" % self.last_run(), self.report())

    def test_a_failure_at_an_unlisted_step_is_a_new_failure_and_the_run_fails(self):
        code, results, _ = self.run_journey("S1.1", "- S1.2: no form. Blocked by #2140.\n")
        self.assertEqual(code, 1)
        self.assertEqual(results, {"S1": ("FAIL", "PASS"), "S2": ("SKIP", "PASS"), "*": ("FAIL", "PASS")})
        self.assertIn("| - | - | - | S1 FAIL at S1.1 |", self.report())

    def test_a_known_step_that_passes_is_fixed(self):
        code, results, _ = self.run_journey(None, "- S1.2: no form. Blocked by #2140.\n")
        self.assertEqual(code, 0)
        self.assertEqual(results, {"S1": ("FIXED", "FAIL"), "S2": ("PASS", "PASS"), "*": ("PASS", "PASS")})
        self.assertIn("| S2 | - | S1 | - |", self.report())

    def test_known_rows_are_not_counted_as_seeded_bugs(self):
        self.run_journey("S1.2", "- S1.2: no form. Blocked by #2140.\n")
        lines = journey.RESULTS.read_text().splitlines()
        item = journey.summarize([dict(zip(lines[0].split("\t"), line.split("\t"))) for line in lines[1:]])[0]
        self.assertEqual((item["seeded"], item["flake"]), (0, 0.0))

    def last_run(self):
        return journey.RESULTS.read_text().splitlines()[-1].split("\t")[journey.COLUMNS.index("run")]


@contextlib.contextmanager
def _yielding(value):
    yield value


SESSION_SWIFT = """nonisolated final class SignInJourneyUITests: XCTestCase {
    func testJourney() throws {
        try session.scenario("S1") {}
        try session.scenario("S2") {}
    }
}
"""

SESSION_TEST = "Test Case '-[MonacoUITests.SignInJourneyUITests testJourney]' %s"


class Session(Output):
    """A journey whose test is one testJourney method: every scenario in one xcodebuild call."""

    def setUp(self):
        super().setUp()
        self.write("docs/journeys/auth/sign-in.md", DOC.replace("actors: [A]", "actors: [A, B]"))
        self.write("ui/SignInJourneyUITests.swift", SESSION_SWIFT)
        self.write("qa/accounts.tsv", "actor\tname\tphone\temail\tcode\nA\tAlfred\t555\ta@b.c\t123456\n"
                   "B\tBea\t556\tb@b.c\t654321\n")
        self.calls = self.write("calls", "")

    def fake_xcodebuild(self, body):
        """A stand-in for xcodebuild that appends its argv and environment to `calls`, then runs body."""
        script = self.write("xcodebuild.sh", "#!/bin/bash\n"
                            "echo \"$* | $TEST_RUNNER_MONACO_QA_B_PHONE | $TEST_RUNNER_MONACO_QA_LAST_SCENARIO\" >> %s\n"
                            "%s\n" % (self.calls, body))
        script.chmod(0o755)
        return script

    def run_journey(self, body, *extra):
        script = self.fake_xcodebuild(body)
        with unittest.mock.patch.object(journey, "journey_backend", lambda *a: _yielding("http://127.0.0.1:8080")), \
                unittest.mock.patch.object(journey, "resolve_simulator", lambda override="", logins=None, profile="": "sim"), \
                unittest.mock.patch.object(journey, "check_simulator_api_environment", lambda sim, url: None), \
                unittest.mock.patch.object(journey, "prepare_logins", lambda *a: None), \
                unittest.mock.patch.object(journey, "reset_journey_simulator", lambda *a: None), \
                unittest.mock.patch.object(journey, "build_label", lambda mutant=None: "abc"), \
                unittest.mock.patch.object(journey, "xcodebuild", lambda sim, *args: [str(script)] + list(args)), \
                redirect_stdout(StringIO()) as printed, unittest.mock.patch("sys.stderr", StringIO()) as err:
            code = journey.main(["run", "auth/sign-in", "--no-build"] + list(extra))
        rows = []
        if journey.RESULTS.exists():
            lines = journey.RESULTS.read_text().splitlines()
            rows = [dict(zip(lines[0].split("\t"), line.split("\t"))) for line in lines[1:]]
        return code, {row["scenario"]: (row["result"], row["failed_step"]) for row in rows}, printed.getvalue() + err.getvalue()

    def canned(self, *lines):
        return "cat <<'LOG'\n%s\nLOG" % "\n".join(lines)

    def test_every_scenario_runs_in_one_call_with_every_actors_login(self):
        code, results, _ = self.run_journey(self.canned(
            SESSION_TEST % "started.",
            "JOURNEYSCENARIO\tbegin\tS1", "JOURNEYSTEP\tbegin\t0\tS1.1", "JOURNEYSTEP\tend\t0\tS1.1\t10",
            "JOURNEYSTEP\tbegin\t0\tS1.2", "JOURNEYSTEP\tend\t0\tS1.2\t10", "JOURNEYSCENARIO\tend\tS1\t1500",
            "JOURNEYSCENARIO\tbegin\tS2", "JOURNEYSTEP\tbegin\t0\tS2.1", "JOURNEYSTEP\tend\t0\tS2.1\t10",
            "JOURNEYSCENARIO\tend\tS2\t500", SESSION_TEST % "passed (2.000 seconds)."))
        self.assertEqual(code, 0)
        self.assertEqual(results, {"S1": ("PASS", ""), "S2": ("PASS", ""), "*": ("PASS", "")})
        calls = self.calls.read_text().splitlines()
        self.assertEqual(len(calls), 1)
        self.assertIn("-only-testing:MonacoUITests/SignInJourneyUITests/testJourney", calls[0])
        self.assertTrue(calls[0].endswith("| 556 | S2"), calls[0])

    def test_a_failed_scenario_fails_at_its_own_step_and_the_rest_are_skipped(self):
        self.write("docs/journeys/auth/sign-in.md", DOC + "\n### S3 Again\n\n| Step | Action |\n| --- | --- |\n| S3.1 | tap |\n")
        self.write("ui/SignInJourney.swift", JOURNEY_SWIFT.replace('step("S2.1")', 'step("S2.1"); step("S3.1")'))
        self.write("ui/SignInJourneyUITests.swift", SESSION_SWIFT.replace(
            '        try session.scenario("S2") {}\n', '        try session.scenario("S2") {}\n        try session.scenario("S3") {}\n'))
        code, results, _ = self.run_journey(self.canned(
            SESSION_TEST % "started.",
            "JOURNEYSCENARIO\tbegin\tS1", "JOURNEYSTEP\tbegin\t0\tS1.1", "JOURNEYSTEP\tend\t0\tS1.1\t10",
            "JOURNEYSCENARIO\tend\tS1\t1500",
            "JOURNEYSCENARIO\tbegin\tS2", "JOURNEYSTEP\tbegin\t0\tS2.1",
            "SignInJourney.swift:12: error: XCTAssertTrue failed - S2.1: no button",
            SESSION_TEST % "failed (2.000 seconds)."))
        self.assertEqual(code, 1)
        self.assertEqual(results, {"S1": ("PASS", ""), "S2": ("FAIL", "S2.1"), "S3": ("SKIP", "after S2 FAIL"),
                                   "*": ("FAIL", "")})
        steps = (journey.OUT.glob("*/steps.tsv").__next__()).read_text()
        self.assertEqual(steps, "S1\tS1.1\t10\t0\n")

    def test_a_skipped_test_is_an_error_not_a_failure(self):
        code, results, _ = self.run_journey(self.canned(
            SESSION_TEST % "started.", "error: no sms login for actor B", SESSION_TEST % "skipped (0.100 seconds)."))
        self.assertEqual(code, 2)
        self.assertEqual(results["S1"], ("ERROR", "the test did not run S1 (skipped)"))
        self.assertEqual(results["S2"], ("SKIP", "after S1 ERROR"))

    def test_the_app_and_the_setup_script_get_the_slots_api_url(self):
        order = journey.ROOT / "order"
        self.write("qa/auth/sign-in.setup.sh", "#!/bin/bash\necho \"setup $MONACO_API_BASE_URL $MONACO_QA_API_BASE_URL\" >> %s\n"
                   % order).chmod(0o755)
        body = "\n".join([
            "d=\"$TEST_RUNNER_MONACO_QA_SETUP_DIR\"",
            "echo \"test $TEST_RUNNER_MONACO_QA_API_BASE_URL\" >> %s" % order,
            "echo \"%s\"" % (SESSION_TEST % "started."),
            "for s in S1 S2; do : > \"$d/$s.request\"; while [ ! -f \"$d/$s.done\" ]; do sleep 0.05; done; done",
            "echo \"%s\"" % (SESSION_TEST % "passed (1.000 seconds)."),
        ])
        self.run_journey(body)
        lines = order.read_text().splitlines()
        self.assertEqual(lines[0], "test http://127.0.0.1:8080")
        self.assertEqual(lines[1:], ["setup http://127.0.0.1:8080 http://127.0.0.1:8080"] * 2)

    def test_scenario_runs_every_scenario_up_to_the_last_named(self):
        code, results, _ = self.run_journey(self.canned(
            SESSION_TEST % "started.", "JOURNEYSCENARIO\tbegin\tS1", "JOURNEYSCENARIO\tend\tS1\t10",
            SESSION_TEST % "passed (1.000 seconds)."), "--scenario", "S1")
        self.assertEqual(code, 0)
        self.assertEqual(sorted(results), ["*", "S1"])
        self.assertTrue(self.calls.read_text().strip().endswith("| S1"))

    def handshake(self, setup_exit):
        order = journey.ROOT / "order"
        self.write("qa/auth/sign-in.setup.sh", "#!/bin/bash\necho \"setup $1 $MONACO_QA_RUN\" >> %s\nexit %d\n"
                   % (order, setup_exit)).chmod(0o755)
        body = "\n".join([
            "d=\"$TEST_RUNNER_MONACO_QA_SETUP_DIR\"",
            "echo \"%s\"" % (SESSION_TEST % "started."),
            "for s in S1 S2; do",
            "  : > \"$d/$s.request\"",
            "  while [ ! -f \"$d/$s.done\" ]; do sleep 0.05; done",
            "  if [ \"$(cat \"$d/$s.done\")\" != ok ]; then echo \"%s\"; exit 1; fi" % (SESSION_TEST % "failed (1.000 seconds)."),
            "  echo \"test $s\" >> %s" % order,
            "  printf 'JOURNEYSCENARIO\\tbegin\\t%s\\nJOURNEYSCENARIO\\tend\\t%s\\t10\\n' $s $s",
            "done",
            "echo \"%s\"" % (SESSION_TEST % "passed (1.000 seconds)."),
        ])
        code, results, printed = self.run_journey(body)
        return code, results, printed, [line.split(" ")[:2] for line in order.read_text().splitlines()]

    def test_the_setup_script_runs_when_the_test_reaches_each_scenario(self):
        code, results, _, order = self.handshake(0)
        self.assertEqual(code, 0)
        self.assertEqual(order, [["setup", "S1"], ["test", "S1"], ["setup", "S2"], ["test", "S2"]])
        self.assertEqual(results["S2"], ("PASS", ""))

    def test_a_failed_setup_stops_the_run_and_names_its_scenario(self):
        code, _, printed, order = self.handshake(3)
        self.assertEqual(code, 2)
        self.assertEqual(order, [["setup", "S1"]])
        self.assertIn("could not set up S1", printed)

    def test_a_hung_session_times_out_in_its_open_scenario(self):
        code, results, printed = self.run_journey(self.canned(
            SESSION_TEST % "started.", "JOURNEYSCENARIO\tbegin\tS1", "JOURNEYSTEP\tbegin\t0\tS1.1") + "\nsleep 60",
            "--timeout", "2")
        self.assertEqual(code, 1)
        self.assertIn("auth/sign-in timed out after 2 s in S1 test", printed)
        self.assertEqual(results, {"S1": ("TIMEOUT", "S1.1"), "S2": ("SKIP", "after S1 TIMEOUT"), "*": ("TIMEOUT", "")})


class MutantCaught(unittest.TestCase):
    def rows(self, *pairs):
        return [{"scenario": s, "result": r, "expected": e} for s, r, e in pairs]

    def test_every_expected_scenario_must_fail_or_be_skipped_after_one_that_failed(self):
        self.assertTrue(journey.mutant_caught(self.rows(("S1", "PASS", "PASS"), ("S2", "FAIL", "FAIL"), ("S3", "SKIP", "FAIL"))))
        self.assertFalse(journey.mutant_caught(self.rows(("S1", "FAIL", "PASS"), ("S2", "SKIP", "FAIL"))))
        self.assertFalse(journey.mutant_caught(self.rows(("S1", "PASS", "FAIL"), ("S2", "FAIL", "FAIL"))))
        self.assertFalse(journey.mutant_caught(self.rows(("S1", "PASS", "PASS"))))


class MutantRunSetup(unittest.TestCase):
    def fake_journey(self, funds):
        patch = unittest.mock.MagicMock()
        patch.stem = "bug"
        return unittest.mock.MagicMock(id="money/withdraw", funds=funds, scenarios=["S1"], mutants=lambda: [patch],
                                       spec=None), patch

    def mutant_env(self, funds, scenarios=("S1",), expect=("S1",), rows=None, free=50 * 10 ** 9, calls=None,
                   order=None, patches=1, setup_error=None, problems=None):
        fake, patch = self.fake_journey(funds)
        if patches > 1:
            stems = ["bug%d" % n for n in range(patches)]
            fake.mutants = lambda: [unittest.mock.MagicMock(stem=stem) for stem in stems]
        trail = order if order is not None else []

        @contextlib.contextmanager
        def backend(slot=None, fake_rpc=False, fake_trade=False):
            trail.append("take_slot")
            try:
                yield "http://x"
            finally:
                trail.append("release_slot")

        fake.scenarios = list(scenarios)
        seen = []

        def run_once(*args, **kwargs):
            trail.append("run_once")
            if setup_error and len(seen) == setup_error[0]:
                seen.append(None)
                raise journey.JourneyError(setup_error[1])
            seen.append(journey.actor_environment({}, "sms", "R"))
            if calls is not None:
                calls.append(args[7])
            return rows or [{"scenario": "S1", "result": "FAIL", "expected": "FAIL", "failed_step": "S1.1"}], "FAIL"

        sh = lambda args, **kw: unittest.mock.Mock(returncode=1 if "--check" in args else 0)
        stack = contextlib.ExitStack()
        with stack:
            for name, value in (("load_journeys", lambda: {"money/withdraw": fake}), ("pick", lambda j, i: fake),
                                ("sh", sh), ("actor_logins", lambda j, a: contextlib.nullcontext({})),
                                ("load_accounts", lambda: {}), ("held_logins", lambda j, a: []),
                                ("use_derived_data", lambda l: None), ("resolve_simulator", lambda s, l, p: "sim"),
                                ("check_simulator_api_environment", lambda s, u: None),
                                ("reset_journey_simulator", lambda *a: None),
                                ("build_mutants", lambda *a: trail.append("build") or {"build_s": {}, "peak": 0}),
                                ("journey_backend", backend),
                                ("mutant_expectation", lambda p: list(expect)), ("run_once", run_once),
                                ("check_preconditions", problems or (lambda *a, **k: [])),
                                ("stamp_file", lambda: unittest.mock.Mock()), ("build_label", lambda *a: "b"),
                                ("stamp", lambda: "T")):
                stack.enter_context(unittest.mock.patch.object(journey, name, value))
            stack.enter_context(unittest.mock.patch.dict(os.environ, {"MONACO_QA_REFUND_ADDRESS": ""}))
            stack.enter_context(unittest.mock.patch.object(journey, "with_dotenv", lambda args: "Pot1"))
            stack.enter_context(unittest.mock.patch.object(
                journey.shutil, "disk_usage", lambda p: unittest.mock.Mock(free=free)))
            args = unittest.mock.Mock(journey="money/withdraw", only=[] if patches > 1 else ["bug"], sim="", runs=1,
                                      slot=None)
            self.code = journey.run_mutants(args)
        return seen[0] if seen else None

    def test_mutants_pass_the_refund_address_the_run_path_passes(self):
        env = self.mutant_env({"A": 5})
        self.assertEqual(env["MONACO_QA_REFUND_ADDRESS"], "Pot1")

    def test_builds_hold_no_slot_and_each_patch_takes_and_releases_one(self):
        order = []
        self.mutant_env({"A": 5}, order=order, patches=2)
        self.assertEqual(order, ["build", "take_slot", "run_once", "release_slot",
                                 "take_slot", "run_once", "release_slot"])

    def test_a_run_stops_after_the_last_scenario_the_patch_names(self):
        calls = []
        self.mutant_env({"A": 5}, ("S1", "S2", "S3", "S4"), ("S1", "S3"), calls=calls)
        self.assertEqual(calls, [["S1", "S2", "S3"]])
        calls.clear()
        self.mutant_env({"A": 5}, ("S1", "S2", "S3", "S4"), ("S2",), calls=calls)
        self.assertEqual(calls, [["S1", "S2"]])

    def test_a_setup_failure_is_not_a_catch_and_a_missed_patch_fails_the_run(self):
        setup = [{"scenario": "S1", "result": "FAIL", "expected": "FAIL", "failed_step": "before S1 began"}]
        with redirect_stdout(StringIO()) as printed:
            self.mutant_env({"A": 5}, rows=setup)
        self.assertIn("setup_failed", printed.getvalue())
        self.assertEqual(self.code, 1)
        missed = [{"scenario": "S1", "result": "PASS", "expected": "FAIL", "failed_step": ""}]
        with redirect_stdout(StringIO()) as printed:
            self.mutant_env({"A": 5}, rows=missed)
        self.assertIn("MISSED", printed.getvalue())
        self.assertEqual(self.code, 1)

    def test_a_failed_setup_marks_that_bug_not_run_and_the_pass_goes_on(self):
        order, recorded = [], []
        with unittest.mock.patch.object(journey, "record", lambda *a, **k: recorded.append(a)), \
                redirect_stdout(StringIO()) as printed:
            self.mutant_env({"A": 5}, order=order, patches=3,
                            setup_error=(1, "apps/x.setup.sh could not set up S2, see log\nsecond line"))
        out = printed.getvalue()
        self.assertEqual(order.count("run_once"), 3)
        self.assertIn("not run: setup apps/x.setup.sh could not set up S2, see log", out)
        self.assertNotIn("second line", out)
        self.assertIn("caught 2 of 3 seeded bugs (1 not run: setup)", out)
        self.assertEqual(self.code, 1)
        self.assertEqual([a[2] for a in recorded], ["T-money-withdraw-bug1-notrun"])
        self.assertTrue(recorded[0][3][0]["failed_step"].startswith("not run: setup "))

    def test_a_failed_precondition_marks_that_bug_not_run_and_the_pass_goes_on(self):
        order = []
        answers = iter([[], ["login C holds a cabal"], []])
        with unittest.mock.patch.object(journey, "record", lambda *a, **k: None), \
                redirect_stdout(StringIO()) as printed:
            self.mutant_env({"A": 5}, order=order, patches=3, problems=lambda *a, **k: next(answers))
        self.assertEqual(order.count("run_once"), 2)
        self.assertIn("not run: setup login C holds a cabal", printed.getvalue())
        self.assertIn("(1 not run: setup)", printed.getvalue())
        self.assertEqual(self.code, 1)

    def test_no_clean_rebuild_follows_the_batch_and_the_clean_stamp_stays(self):
        stamp_calls = []
        with unittest.mock.patch.object(journey, "build", lambda *a: stamp_calls.append("build")), \
                unittest.mock.patch.object(journey, "stamp_file", lambda: stamp_calls.append("stamp")), \
                redirect_stdout(StringIO()) as printed:
            self.mutant_env({"A": 5})
        self.assertEqual(stamp_calls, [])
        self.assertEqual(self.code, 0)
        self.assertIn("mutants folder peak", printed.getvalue())

    def test_the_batch_is_refused_below_20_gb_free(self):
        with self.assertRaisesRegex(journey.JourneyError, "only 5.0 GB is free"):
            self.mutant_env({"A": 5}, free=5 * 10 ** 9)

    def test_one_slot_hold_builds_every_patch(self):
        runs, envs = [], []
        done = unittest.mock.Mock(returncode=0)

        def sh(args, **kw):
            runs.append(args)
            envs.append(kw.get("env", {}))
            for index, word in enumerate(args):
                if word == "_build-mutants":
                    Path(args[index + 4]).write_text('{"build_s": {"a": 1.0}, "peak": 5}')
            return done

        with unittest.mock.patch.object(journey, "sh", sh):
            data = journey.build_mutants("sim", [Path("a.patch"), Path("b.patch")], Path("m"))
        locked = [r for r in runs if r[:2] == ["scripts/qa/xcode-lock.sh", "xcode"]]
        self.assertEqual(len(locked), 1)
        self.assertEqual([r for r in locked[0] if r.endswith(".patch")], ["a.patch", "b.patch"])
        self.assertEqual(data["peak"], 5)
        self.assertEqual(envs[-1].get("MONACO_LOCK_HOLD"), "2400")

    def test_a_run_that_ends_before_s1_is_setup_failed_not_caught(self):
        rows = [{"scenario": "S1", "result": "FAIL", "expected": "FAIL", "failed_step": "before S1 began"},
                {"scenario": "S2", "result": "SKIP", "expected": "FAIL", "failed_step": "after S1 FAIL"}]
        self.assertTrue(journey.setup_failed(rows))
        self.assertFalse(journey.mutant_caught(rows))
        started = [dict(rows[0], failed_step="S1.1"), rows[1]]
        self.assertFalse(journey.setup_failed(started))
        self.assertTrue(journey.mutant_caught(started))


@contextlib.contextmanager
def _patched(*patches):
    with contextlib.ExitStack() as stack:
        for patch in patches:
            stack.enter_context(patch)
        yield


class Stamp(Output):
    def setUp(self):
        super().setUp()
        self.write("apps/mobile/App.swift", "let a = 1\n")
        self.write("docs/notes.md", "notes\n")
        for args in (["init", "-q"], ["add", "."], ["-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "x"]):
            subprocess.run(["git"] + args, cwd=str(journey.ROOT), check=True)

    def test_the_stamp_changes_only_with_the_app_sources(self):
        first = journey.build_stamp()
        self.assertEqual(journey.build_stamp(), first)
        self.write("docs/notes.md", "other notes\n")
        self.assertEqual(journey.build_stamp(), first)
        self.write("apps/mobile/App.swift", "let a = 2\n")
        edited = journey.build_stamp()
        self.assertNotEqual(edited, first)
        self.write("apps/mobile/New.swift", "let b = 1\n")
        added = journey.build_stamp()
        self.assertNotEqual(added, edited)
        self.write("apps/mobile/New.swift", "let b = 2\n")
        self.assertNotEqual(journey.build_stamp(), added)

    def fake_build(self, built, signed=True):
        def build(sim, log, current=None):
            built.append((journey.DERIVED.name, current))
            products = journey.DERIVED / "Build" / "Products"
            products.mkdir(parents=True, exist_ok=True)
            (products / "Monaco.xctestrun").write_text("")
            journey.stamp_file().write_text(current + "\n")
        return _patched(
            unittest.mock.patch.object(journey, "build", build),
            unittest.mock.patch.object(journey, "app_is_signed", lambda: signed))

    def test_a_matching_stamp_and_test_bundle_skip_the_build(self):
        built = []
        with self.fake_build(built), redirect_stdout(StringIO()) as printed:
            journey.ensure_build("sim", journey.OUT / "build.log")
            journey.ensure_build("sim", journey.OUT / "build.log")
            journey.ensure_build("sim", journey.OUT / "build.log", rebuild=True)
        self.assertEqual([name for name, _ in built], ["DerivedData", "DerivedData"])
        self.assertIn("reusing build %s" % journey.build_stamp()[:8], printed.getvalue())

    def test_the_journey_builds_into_the_folder_stage_0_builds(self):
        self.assertEqual(journey.shared_build_folder(), journey.ROOT / ".build" / "DerivedData")

    def test_an_unsigned_build_of_the_same_sources_is_built_again(self):
        built = []
        with self.fake_build(built, signed=False), redirect_stdout(StringIO()):
            journey.ensure_build("sim", journey.OUT / "build.log")
            journey.ensure_build("sim", journey.OUT / "build.log")
        self.assertEqual(len(built), 2)

    def test_the_clone_waits_for_an_xcodebuild_that_holds_the_folder_and_takes_a_dead_ones_lock(self):
        folder = journey.ROOT / ".build" / "DerivedData"
        lock = Path(str(folder) + ".lock")
        lock.mkdir(parents=True)
        (lock / "pid").write_text("%d\n" % os.getpid())
        waits = []

        def sleep(seconds):
            waits.append(seconds)
            (lock / "pid").write_text("999999\n")

        with redirect_stdout(StringIO()), journey.holding_derived_folder(folder, sleep=sleep):
            self.assertEqual((lock / "pid").read_text().strip(), str(os.getpid()))
        self.assertEqual(waits, [journey.LOCK_POLL_SECONDS])
        self.assertFalse(lock.exists())

    def test_two_login_sets_on_one_stamp_build_once_and_each_gets_its_own_clone(self):
        built = []
        with self.fake_build(built), redirect_stdout(StringIO()):
            for logins in ({"A": "A"}, {"A": "A", "B": "B"}, {"A": "A"}):
                journey.use_derived_data(logins)
                journey.ensure_build("sim", journey.OUT / "build.log")
        self.assertEqual(len(built), 1)
        for name in ("derived-A", "derived-A-B"):
            self.assertTrue((journey.OUT / name / "Build/Products/Monaco.xctestrun").exists())
            self.assertEqual((journey.OUT / name / "build.stamp").read_text().strip(), journey.build_stamp())

    def test_a_changed_source_rebuilds_and_refreshes_every_clone(self):
        built = []
        with self.fake_build(built), redirect_stdout(StringIO()):
            journey.use_derived_data({"A": "A"})
            journey.ensure_build("sim", journey.OUT / "build.log")
            self.write("apps/mobile/App.swift", "let a = 3\n")
            journey.ensure_build("sim", journey.OUT / "build.log")
        self.assertEqual(len(built), 2)
        self.assertEqual((journey.OUT / "derived-A" / "build.stamp").read_text().strip(), journey.build_stamp())

    def test_a_patched_tree_has_another_stamp_so_a_mutant_never_reuses_the_clean_build(self):
        clean = journey.build_stamp()
        self.write("apps/mobile/App.swift", "let a = 99\n")
        self.assertNotEqual(journey.build_stamp(), clean)

    def test_the_mutants_path_never_calls_ensure_build(self):
        self.assertNotIn("ensure_build", inspect.getsource(journey.run_mutants))
        self.assertNotIn("ensure_build", inspect.getsource(journey.cmd_build_mutants))


class All(Output):
    def journeys(self):
        self.write("docs/journeys/cabals/join.md", DOC.replace("auth/sign-in", "cabals/join").replace(
            "requires: []", "requires: [cabals/create]"))
        self.write("docs/journeys/cabals/create.md", DOC.replace("auth/sign-in", "cabals/create").replace(
            "requires: []", "requires: [auth/sign-in]"))
        self.write("docs/journeys/cabals/fund.md", DOC.replace("auth/sign-in", "cabals/fund").replace(
            "requires: []", "requires: []\nfunds:\n  A: 5"))

    def run_main(self, argv, refund="", pot="", engines=None):
        backends, builds, ran = [], [], []

        @contextlib.contextmanager
        def backend(*a):
            backends.append(1)
            yield "http://127.0.0.1:8080"

        def run_once(journey_, *a, **k):
            ran.append(journey_.id)
            if engines is not None:
                engines.append(os.environ.get("TRADE_ENGINE"))
            return [], "PASS"

        with unittest.mock.patch.object(journey, "journey_backend", backend), \
                unittest.mock.patch.object(journey, "resolve_simulator", lambda override="", logins=None, profile="": "sim"), \
                unittest.mock.patch.object(journey, "check_simulator_api_environment", lambda sim, url: None), \
                unittest.mock.patch.object(journey, "prepare_logins", lambda *a: None), \
                unittest.mock.patch.object(journey, "check_preconditions", lambda *a, **k: []), \
                unittest.mock.patch.object(journey, "reset_journey_simulator", lambda *a: None), \
                unittest.mock.patch.object(journey, "ensure_build", lambda sim, log, rebuild=False: builds.append(sim)), \
                unittest.mock.patch.object(journey, "run_once", run_once), \
                unittest.mock.patch.object(journey, "build_label", lambda mutant=None: "abc"), \
                unittest.mock.patch.dict(os.environ, {"MONACO_QA_REFUND_ADDRESS": refund}), \
                unittest.mock.patch.object(journey, "with_dotenv", lambda args: pot), \
                redirect_stdout(StringIO()) as printed, unittest.mock.patch("sys.stderr", StringIO()) as err:
            code = journey.main(argv)
        return code, ran, (len(backends), len(builds)), printed.getvalue() + err.getvalue()

    def test_every_journey_runs_after_what_it_requires_on_one_build_and_one_backend(self):
        self.journeys()
        code, ran, counts, out = self.run_main(["run", "--all"])
        self.assertEqual(code, 0)
        self.assertEqual(ran, ["auth/sign-in", "cabals/create", "cabals/join"])
        self.assertEqual(counts, (1, 1))
        self.assertIn("cabals/fund SKIP funds", out)

    def test_all_skips_funds_journeys_even_when_the_qa_pot_resolves(self):
        self.journeys()
        code, ran, _, out = self.run_main(["run", "--all"], pot="Pot1")
        self.assertEqual(code, 0)
        self.assertEqual(ran, ["auth/sign-in", "cabals/create", "cabals/join"])
        self.assertIn("cabals/fund SKIP funds", out)

    def test_all_runs_funds_journeys_on_the_live_engine_when_the_refund_address_is_exported(self):
        self.journeys()
        engines = []
        code, ran, _, out = self.run_main(["run", "--all"], refund="Phantom1", engines=engines)
        self.assertEqual(code, 0)
        self.assertIn("cabals/fund", ran)
        self.assertNotIn("SKIP funds", out)
        self.assertEqual(set(engines), {"live"})

    def test_named_journeys_run_in_requires_order_on_one_build_and_one_backend(self):
        self.journeys()
        code, ran, counts, _ = self.run_main(["run", "cabals/join", "auth/sign-in", "cabals/create"])
        self.assertEqual(code, 0)
        self.assertEqual(ran, ["auth/sign-in", "cabals/create", "cabals/join"])
        self.assertEqual(counts, (1, 1))

    def test_scenario_needs_exactly_one_journey(self):
        self.journeys()
        code, ran, _, out = self.run_main(["run", "auth/sign-in", "cabals/join", "--scenario", "S1"])
        self.assertEqual((code, ran), (2, []))
        self.assertIn("--scenario names one journey's scenarios: name exactly one journey", out)

    def test_run_needs_a_journey_or_all(self):
        with redirect_stdout(StringIO()), unittest.mock.patch("sys.stderr", StringIO()) as err:
            self.assertEqual(journey.main(["run"]), 2)
        self.assertIn("name one or more journeys, or pass --all", err.getvalue())


class Seed(unittest.TestCase):
    """scripts/qa/seed.sh against a local server, with the token stubbed."""

    def setUp(self):
        import http.server

        limited = b'{"code":"rate_limited","retryable":%s}'
        replies = {
            "/ok": [(200, {}, b'{"id":"c1"}')],
            "/limited": [(429, {}, limited % b"false")],
            "/flaky": [(429, {"Retry-After": "0"}, limited % b"true"), (429, {}, limited % b"true"),
                       (200, {}, b'{"id":"c1"}')],
        }
        self.calls = calls = {}

        class Handler(http.server.BaseHTTPRequestHandler):
            def do_GET(self):
                n = calls[self.path] = calls.get(self.path, 0) + 1
                script = replies.get(self.path, [(404, {}, b'{"error":"no cabal"}')])
                code, headers, body = script[min(n, len(script)) - 1]
                self.send_response(code)
                for name, value in headers.items():
                    self.send_header(name, value)
                self.end_headers()
                self.wfile.write(body)

            def log_message(self, *args):
                pass

        self.server = http.server.HTTPServer(("127.0.0.1", 0), Handler)
        threading.Thread(target=self.server.serve_forever, daemon=True).start()

    def tearDown(self):
        self.server.shutdown()
        self.server.server_close()

    def qa_api(self, path, shell="bash"):
        script = 'source scripts/qa/seed.sh; qa_token() { echo tok; }; sleep() { :; }; qa_api A GET %s' % path
        env = dict(os.environ, MONACO_API_BASE_URL="http://127.0.0.1:%d" % self.server.server_port)
        return subprocess.run([shell, "-c", script], cwd=str(Path(__file__).resolve().parents[2]), env=env,
                              stdout=subprocess.PIPE, stderr=subprocess.PIPE, universal_newlines=True)

    def test_a_2xx_prints_the_body(self):
        done = self.qa_api("/ok")
        self.assertEqual((done.returncode, done.stdout), (0, '{"id":"c1"}'))

    def test_a_4xx_fails_and_prints_the_body(self):
        done = self.qa_api("/missing")
        self.assertNotEqual(done.returncode, 0)
        self.assertIn("HTTP 404", done.stderr)
        self.assertIn('{"error":"no cabal"}', done.stderr)

    def test_a_retryable_429_is_retried_until_it_succeeds(self):
        shells = ["bash"] + (["zsh"] if shutil.which("zsh") else [])
        for shell in shells:
            with self.subTest(shell=shell):
                self.calls.clear()
                done = self.qa_api("/flaky", shell=shell)
                self.assertEqual((done.returncode, done.stdout, self.calls["/flaky"]), (0, '{"id":"c1"}', 3))
                self.assertEqual(done.stderr.splitlines(), [
                    "qa_api: 429 rate_limited on GET /flaky, retry 1 in 0s",
                    "qa_api: 429 rate_limited on GET /flaky, retry 2 in 2s",
                ])

    def test_a_429_that_is_not_retryable_fails_at_once(self):
        done = self.qa_api("/limited")
        self.assertNotEqual(done.returncode, 0)
        self.assertEqual(self.calls["/limited"], 1)
        self.assertNotIn("rate_limited on GET", done.stderr)
        self.assertIn("HTTP 429", done.stderr)

    @unittest.skipUnless(shutil.which("zsh"), "zsh is not installed")
    def test_qa_api_works_when_sourced_from_zsh(self):
        done = self.qa_api("/ok", shell="zsh")
        self.assertEqual((done.returncode, done.stdout, done.stderr), (0, '{"id":"c1"}', ""))

    def test_monacoctl_runs_without_the_runners_api_base_url(self):
        root = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, str(root))
        for name, body in (("bin/monacoctl", 'env | grep "^MONACO_" || true; echo ran'),
                           ("scripts/with-dotenv-local.sh", 'exec "$@"')):
            (root / name).parent.mkdir(parents=True, exist_ok=True)
            (root / name).write_text("#!/bin/bash\n%s\n" % body)
            (root / name).chmod(0o755)
        (root / "scripts/qa").mkdir(parents=True, exist_ok=True)
        shutil.copy(str(journey.ROOT / "scripts/qa/monacoctl.sh"), str(root / "scripts/qa/monacoctl.sh"))
        script = 'source scripts/qa/seed.sh; QA_ROOT=%s; qa_flow_seed f ok' % root
        env = dict(os.environ, MONACO_API_BASE_URL="http://127.0.0.1:1")
        done = subprocess.run(["bash", "-c", script], cwd=str(journey.ROOT), env=env,
                              stdout=subprocess.PIPE, stderr=subprocess.PIPE, universal_newlines=True)
        self.assertEqual((done.returncode, done.stdout), (0, "ran\n"), done.stderr)


class StartBackend(unittest.TestCase):
    def env_of_started_backend(self, environ):
        started = []

        def popen(args, env, **kwargs):
            started.append(env)
            raise StopIteration

        with tempfile.TemporaryDirectory() as out, unittest.mock.patch.object(journey, "OUT", Path(out)), \
                unittest.mock.patch.dict(os.environ, environ, clear=True), \
                unittest.mock.patch.object(journey, "apply_event_streams", lambda log, url=None: None), \
                unittest.mock.patch.object(journey.subprocess, "Popen", popen), redirect_stdout(StringIO()):
            with self.assertRaises(StopIteration):
                journey.start_backend("http://127.0.0.1:8080")
        return started[0]

    def test_fake_rpc_points_the_backend_at_the_fakes_server(self):
        env = self.env_of_started_backend({"QA_FAKE_RPC": "1", "SOLANA_RPC_URL": "https://mainnet"})
        self.assertEqual(env["SOLANA_RPC_URL"], "http://127.0.0.1:8099/rpc/")
        env = self.env_of_started_backend({"QA_FAKE_RPC": "1", "QA_FAKES_URL": "http://127.0.0.1:9000"})
        self.assertEqual(env["SOLANA_RPC_URL"], "http://127.0.0.1:9000/rpc/")

    def test_fake_trade_points_jupiter_and_privy_wallets_at_the_fakes_and_leaves_privy_auth(self):
        reply = unittest.mock.MagicMock()
        reply.__enter__.return_value = StringIO('{"key": "wallet-auth:fixture", "key_id": "k"}')
        seen = []

        def urlopen(url, timeout=None):
            seen.append(url)
            return reply

        with unittest.mock.patch.object(journey.urllib.request, "urlopen", urlopen):
            env = self.env_of_started_backend({
                "QA_FAKE_RPC": "1", "QA_FAKE_TRADE": "1", "QA_FAKES_URL": "http://127.0.0.1:9000",
                "PRIVY_BASE_URL": "https://api.privy.io", "MONACO_JUPITER_SWAP_BASE_URL": "https://api.jup.ag"})
        self.assertEqual(seen, ["http://127.0.0.1:9000/_privy_authorization_key"])
        self.assertEqual(env["MONACO_JUPITER_SWAP_BASE_URL"], "http://127.0.0.1:9000/jupiter/swap/v2")
        self.assertEqual(env["MONACO_JUPITER_PRICE_BASE_URL"], "http://127.0.0.1:9000/jupiter/price/v3")
        self.assertEqual(env["PRIVY_WALLET_BASE_URL"], "http://127.0.0.1:9000/privy")
        self.assertEqual(env["PRIVY_AUTHORIZATION_PRIVATE_KEY"], "wallet-auth:fixture")
        self.assertEqual(env["PRIVY_BASE_URL"], "https://api.privy.io")
        self.assertEqual(env["TRADE_ENGINE"], "live")

    def test_fake_trade_refuses_to_start_when_a_trading_url_is_not_on_the_fakes(self):
        fakes = "http://127.0.0.1:9000"
        good = {name: fakes + "/x" for name in journey.FAKE_TRADE_URLS}
        journey.require_fakes_only(good, fakes)
        for name in journey.FAKE_TRADE_URLS:
            for env in (dict(good, **{name: "https://real.example"}), {k: v for k, v in good.items() if k != name}):
                with self.assertRaisesRegex(journey.JourneyError, name):
                    journey.require_fakes_only(env, fakes)

    def test_fake_trade_without_the_fake_rpc_starts_nothing(self):
        reply = unittest.mock.MagicMock()
        reply.__enter__.return_value = StringIO('{"key": "k"}')
        popen = unittest.mock.Mock()
        with unittest.mock.patch.object(journey.urllib.request, "urlopen", lambda url, timeout=None: reply), \
                unittest.mock.patch.object(journey.subprocess, "Popen", popen):
            with self.assertRaisesRegex(journey.JourneyError, "SOLANA_RPC_URL"):
                self.env_of_started_backend({"QA_FAKE_TRADE": "1", "SOLANA_RPC_URL": "https://mainnet"})
        popen.assert_not_called()

    def test_fake_rpc_alone_leaves_jupiter_and_privy_wallets_live(self):
        env = self.env_of_started_backend({"QA_FAKE_RPC": "1", "MONACO_JUPITER_SWAP_BASE_URL": "https://api.jup.ag"})
        self.assertEqual(env["MONACO_JUPITER_SWAP_BASE_URL"], "https://api.jup.ag")
        self.assertNotIn("PRIVY_WALLET_BASE_URL", env)

    def test_without_fake_rpc_the_backend_keeps_its_rpc(self):
        env = self.env_of_started_backend({"SOLANA_RPC_URL": "https://mainnet"})
        self.assertEqual(env["SOLANA_RPC_URL"], "https://mainnet")


class XcodeVersionWarning(unittest.TestCase):
    def warning_for(self, pinned, installed):
        with tempfile.TemporaryDirectory() as repo:
            if pinned is not None:
                (Path(repo) / ".xcode-version").write_text(pinned + "\n")
            with unittest.mock.patch.object(journey, "ROOT", Path(repo)):
                return journey.xcode_version_warning(lambda: installed)

    def test_a_different_xcode_returns_the_message(self):
        self.assertEqual(
            self.warning_for("26.4.1", "26.3"),
            "Xcode 26.3 is installed; the repo pins 26.4.1 (.xcode-version). "
            "Install it from Apple's developer downloads or with xcodes.")

    def test_the_pinned_xcode_returns_none(self):
        self.assertIsNone(self.warning_for("26.4.1", "26.4.1"))

    def test_a_missing_pin_or_missing_xcodebuild_returns_none(self):
        self.assertIsNone(self.warning_for(None, "26.3"))
        self.assertIsNone(self.warning_for("26.4.1", None))


class OnceBuilder(unittest.TestCase):
    def run_builder(self, no_build):
        calls = []
        saved = journey.DERIVED
        args = types.SimpleNamespace(no_build=no_build, rebuild=False)
        try:
            with unittest.mock.patch.object(journey, "ensure_build", lambda *a: calls.append(journey.DERIVED.name)):
                builder = journey.once_builder(args)
                for logins in ({"A": "A"}, {"A": "A", "B": "B"}, {"A": "A"}, {"A": "A", "B": "B"}):
                    journey.use_derived_data(logins)
                    builder("sim")
        finally:
            journey.DERIVED = saved
        return calls

    def test_builds_once_per_derived_folder(self):
        self.assertEqual(self.run_builder(False), ["derived-A", "derived-A-B"])

    def test_no_build_never_builds(self):
        self.assertEqual(self.run_builder(True), [])


class Phases(unittest.TestCase):
    def setUp(self):
        journey.PHASES.clear()
        del journey.TAKEN[:]
        self.addCleanup(journey.PHASES.clear)
        self.addCleanup(lambda: journey.TAKEN.__delitem__(slice(None)))
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)

    def row(self, **values):
        base = dict.fromkeys(journey.COLUMNS, "")
        base.update({"journey": "j", "driver": "xcuitest", "build": "abc", "expected": "PASS", "truth": "none",
                     "scenario": "*", "result": "PASS", "run": "r1"})
        base.update(values)
        return base

    def test_the_phase_columns_come_last_in_order(self):
        self.assertEqual(journey.COLUMNS[-9:], [
            "build_s", "build_queue_s", "boot_s", "boot_queue_s", "backend_s", "setup_s", "switch_s", "truth_s",
            "uninstall_s"])
        self.assertEqual(journey.COLUMNS[:14][-2:], ["failed_step", "log"])

    def test_an_old_row_without_the_phase_columns_is_skipped_in_the_medians(self):
        old = {key: self.row(wall_s="10")[key] for key in journey.COLUMNS[:14]}
        item = journey.summarize([old])[0]
        self.assertEqual(item["wall_s"], 10.0)
        self.assertEqual(set(item["phases"].values()), {None})
        self.assertEqual(journey.total_seconds(item), 10.0)
        self.assertEqual(journey.build_lines([old]), [])

    def test_the_report_takes_the_median_of_each_phase_and_adds_them_to_the_wall(self):
        rows = [self.row(wall_s="10", build_s="30", boot_s="2", switch_s="1"),
                self.row(wall_s="20", build_s="10", boot_s="4", switch_s="3"),
                self.row(wall_s="30", build_s="20", boot_s="6", switch_s="5"),
                self.row(wall_s="", result="FAIL")]
        item = journey.summarize(rows)[0]
        self.assertEqual((item["phases"]["build_s"], item["phases"]["boot_s"], item["phases"]["switch_s"]), (20, 4, 3))
        self.assertIsNone(item["phases"]["truth_s"])
        self.assertEqual(journey.total_seconds(item), 20 + 20 + 4)
        self.assertIsNone(journey.total_seconds({"wall_s": None, "phases": {}}))

    def test_the_report_names_a_built_and_a_reused_build(self):
        rows = [self.row(run="a", build_s="12.0", build_queue_s="3.0"), self.row(run="b", build_s="0.0", build_queue_s="0.0"),
                self.row(run="c", build_s="5.0"), self.row(run="d", scenario="S1", build_s="9.0")]
        self.assertEqual(journey.build_lines(rows), [
            "a: built in 12.0 s after 3.0 s in the queue", "b: reused build", "c: built in 5.0 s after 0.0 s in the queue"])
        self.assertEqual(len(journey.build_lines(rows * 8, limit=2)), 2)

    def test_the_report_command_prints_the_phase_table(self):
        with unittest.mock.patch.object(journey, "RESULTS", Path(self.tmp.name) / "results.tsv"):
            journey.RESULTS.write_text("\t".join(journey.COLUMNS) + "\n" + "\t".join(
                self.row(wall_s="10", build_s="2.0", build_queue_s="1.0").get(c, "") for c in journey.COLUMNS) + "\n")
            with redirect_stdout(StringIO()) as printed:
                self.assertEqual(journey.cmd_report(types.SimpleNamespace(evidence=None)), 0)
        self.assertIn("Median build s", printed.getvalue())
        self.assertIn("r1: built in 2.0 s after 1.0 s in the queue", printed.getvalue())

    def test_the_lock_wait_is_read_from_the_file_the_lock_script_appends_to(self):
        path = Path(self.tmp.name) / "waited"
        self.assertEqual(journey.read_lock_waited(path), 0)
        path.write_text("3\n4\n")
        self.assertEqual(journey.read_lock_waited(path), 7)
        path.write_text("x\n")
        self.assertEqual(journey.read_lock_waited(path), 0)

    def build_with(self, waited, code=0):
        def fake_sh(args, **kwargs):
            if "MONACO_LOCK_WAITED" in kwargs.get("env", {}):
                Path(kwargs["env"]["MONACO_LOCK_WAITED"]).write_text(waited)
                return types.SimpleNamespace(returncode=code)
            return types.SimpleNamespace(returncode=0, stdout="")
        with unittest.mock.patch.object(journey, "sh", fake_sh), \
                unittest.mock.patch.object(journey, "DERIVED", Path(self.tmp.name) / "derived"), \
                unittest.mock.patch.object(journey, "xcodebuild", lambda sim, *extra: ["xcodebuild"]), \
                redirect_stdout(StringIO()):
            journey.build("sim", Path(self.tmp.name) / "build.log", "stamp")

    def test_the_build_queue_comes_from_the_lock_wait_file_and_leaves_the_build_time(self):
        self.build_with("2\n")
        self.assertEqual(journey.PHASES["build_queue_s"], 2.0)
        self.assertGreaterEqual(journey.PHASES["build_s"], 0.0)
        self.assertLess(journey.PHASES["build_s"], 2.0)

    def test_a_failed_build_still_records_its_time_and_raises(self):
        with self.assertRaises(journey.JourneyError):
            self.build_with("", code=65)
        self.assertEqual(journey.PHASES["build_queue_s"], 0.0)

    def test_a_reused_build_takes_no_build_time_and_no_queue(self):
        with unittest.mock.patch.object(journey, "build_stamp", lambda: "abcdef123456"), \
                unittest.mock.patch.object(journey, "build_is_current", lambda current: True), \
                redirect_stdout(StringIO()):
            self.assertFalse(journey.ensure_build("sim", Path("x.log")))
        self.assertEqual((journey.PHASES["build_s"], journey.PHASES["build_queue_s"]), (0.0, 0.0))

    def test_switch_seconds_sum_the_p1_steps_only(self):
        timings = [("P1", 1500, 0), ("S1.1", 500, 1), ("P1.2", 250, 1), ("XP1", 9000, 0)]
        self.assertEqual(journey.switch_seconds(timings), 1.75)
        self.assertEqual(journey.switch_seconds([]), 0.0)

    def test_a_scenario_row_carries_its_switch_time_and_a_skipped_one_has_none(self):
        output = "\n".join([
            "Test Case '-[MonacoUITests.T testJourney]' started.", "JOURNEYSCENARIO\tbegin\tS1",
            "JOURNEYSTEP\tbegin\t0\tP1", "JOURNEYSTEP\tend\t0\tP1\t1500", "JOURNEYSCENARIO\tend\tS1\t2000",
            "Test Case '-[MonacoUITests.T testJourney]' failed (2.000 seconds)."])
        rows, _ = journey.session_rows(["S1", "S2"], output, "MonacoUITests/testJourney", None)
        self.assertEqual(rows[0]["switch_s"], "1.5")
        self.assertEqual(rows[1]["switch_s"], "")

    def test_the_timers_add_up_and_the_summary_row_takes_them_once(self):
        with journey.phase("truth_s"):
            pass
        journey.add_phase("truth_s", 1.0)
        self.assertGreaterEqual(float(journey.take_phases()["truth_s"]), 1.0)
        self.assertEqual(journey.take_phases(), {})
        journey.add_phase("setup_s", 2.0)
        with redirect_stdout(StringIO()) as printed:
            journey.print_phase_summary()
        self.assertIn("truth", printed.getvalue())
        self.assertIn("setup 2.0 s", printed.getvalue())

    def test_no_measurement_prints_nothing(self):
        self.assertIsNone(journey.phase_line({}))
        with redirect_stdout(StringIO()) as printed:
            journey.print_phase_summary()
        self.assertEqual(printed.getvalue(), "")

    def test_an_old_results_header_is_widened_and_a_current_one_is_left(self):
        with unittest.mock.patch.object(journey, "RESULTS", Path(self.tmp.name) / "results.tsv"):
            old = "\t".join(journey.COLUMNS[:14])
            journey.RESULTS.write_text(old + "\nrow\n")
            journey.upgrade_header()
            self.assertEqual(journey.RESULTS.read_text(), "\t".join(journey.COLUMNS) + "\nrow\n")
            journey.upgrade_header()
            self.assertEqual(journey.RESULTS.read_text(), "\t".join(journey.COLUMNS) + "\nrow\n")
            journey.RESULTS.write_text(old)
            journey.upgrade_header()
            self.assertEqual(journey.RESULTS.read_text(), "\t".join(journey.COLUMNS) + "\n")

    def test_a_simulator_boot_splits_the_queue_from_the_boot(self):
        def fake_take(udid):
            journey.add_phase("boot_queue_s", 5.0)
        with unittest.mock.patch.object(journey, "take_sim_slot", fake_take), \
                unittest.mock.patch.object(journey, "sh", lambda *a, **k: types.SimpleNamespace(returncode=0)), \
                unittest.mock.patch.object(journey, "BOOTED", []):
            journey.boot_simulator("udid")
        self.assertEqual(journey.PHASES["boot_queue_s"], 5.0)
        self.assertLess(journey.PHASES["boot_s"], 1.0)
        with unittest.mock.patch.object(journey, "take_sim_slot", fake_take), \
                unittest.mock.patch.object(journey, "release_sim_slot"), \
                unittest.mock.patch.object(journey, "sh", side_effect=OSError("boom")):
            with self.assertRaises(OSError):
                journey.boot_simulator("udid2")


class Dev(unittest.TestCase):
    def row(self, **values):
        base = dict.fromkeys(journey.COLUMNS + journey.PHASE_COLUMNS, "")
        base.update({"journey": "auth/sign-in", "driver": "xcuitest", "build": "abc", "expected": "PASS", "truth": "none",
                     "scenario": "*", "result": "PASS", "wall_s": "10", "build_s": "5", "run": "r"})
        base.update(values)
        return base

    def test_a_dev_row_changes_no_count_and_no_latest_outcome(self):
        clean = [self.row(run="r1"), self.row(run="r1", scenario="S1")]
        dev = [self.row(run="r2", build="dev", result="FAIL", wall_s="500"),
               self.row(run="r2", build="dev", scenario="S1", result="FAIL", failed_step="S1.2")]
        self.assertEqual(journey.summarize(clean + dev), journey.summarize(clean))
        self.assertEqual(journey.latest_outcomes(clean + dev, {}), journey.latest_outcomes(clean, {}))
        self.assertEqual(journey.build_lines(clean + dev), journey.build_lines(clean))
        self.assertEqual(journey.summarize(dev), [])
        self.assertEqual(journey.latest_outcomes(dev, {}), [])

    def test_changed_paths_names_what_was_added_removed_or_edited(self):
        before = {"a": (1, 1), "b": (1, 1), "c": (1, 1)}
        after = {"a": (1, 1), "b": (2, 1), "d": (1, 1)}
        self.assertEqual(journey.changed_paths(before, after), ["b", "c", "d"])

    def test_only_the_ui_test_bundle_changing_is_a_test_only_edit(self):
        tests = "apps/mobile/MonacoUITests/ui/A.swift"
        self.assertTrue(journey.only_tests_changed([tests]))
        self.assertFalse(journey.only_tests_changed([tests, "apps/mobile/Monaco/App.swift"]))
        self.assertFalse(journey.only_tests_changed([]))

    def test_the_snapshot_sees_an_edit_to_a_source_and_skips_build_folders(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            (root / "apps" / "mobile" / ".build").mkdir(parents=True)
            (root / "apps" / "mobile" / "A.swift").write_text("a")
            (root / "apps" / "mobile" / ".build" / "x.o").write_text("x")
            before = journey.source_snapshot(root, ("apps/mobile", "packages/mobile-core"))
            self.assertEqual(list(before), ["apps/mobile/A.swift"])
            (root / "apps" / "mobile" / "A.swift").write_text("ab")
            after = journey.source_snapshot(root, ("apps/mobile", "packages/mobile-core"))
            self.assertEqual(journey.changed_paths(before, after), ["apps/mobile/A.swift"])

    def test_the_failure_screenshot_and_hierarchy_come_from_the_manifest(self):
        manifest = [{"testIdentifier": "T/testJourney", "attachments": [
            {"exportedFileName": "early.png", "suggestedHumanReadableName": "Screenshot_1.png"},
            {"exportedFileName": "shot.jpeg", "suggestedHumanReadableName": "Screenshot_2.jpeg", "isAssociatedWithFailure": True},
            {"exportedFileName": "tree.txt", "suggestedHumanReadableName": "App UI hierarchy_3.txt", "isAssociatedWithFailure": True},
            {"exportedFileName": "other.png", "suggestedHumanReadableName": "Screenshot_4.png"}]}]
        self.assertEqual(journey.failure_attachments(manifest), ("shot.jpeg", "tree.txt"))
        self.assertEqual(journey.failure_attachments([]), ("", ""))

    def test_the_capture_taken_at_the_failure_beats_a_later_failure_flagged_stale_one(self):
        manifest = [{"testIdentifier": "T/testJourney", "attachments": [
            {"exportedFileName": "now.png", "suggestedHumanReadableName": "failure-screenshot_1_ABC.png"},
            {"exportedFileName": "now.txt", "suggestedHumanReadableName": "failure-hierarchy_2_ABC.txt"},
            {"exportedFileName": "stale.png", "suggestedHumanReadableName": "Screenshot_3.png", "isAssociatedWithFailure": True},
            {"exportedFileName": "stale.txt", "suggestedHumanReadableName": "App UI hierarchy_4.txt", "isAssociatedWithFailure": True},
            {"exportedFileName": "manual.png", "suggestedHumanReadableName": "S1-A-search_5.png"}]}]
        self.assertEqual(journey.failure_attachments(manifest), ("now.png", "now.txt"))

    def test_attachments_listed_newest_first_are_picked_by_timestamp(self):
        manifest = [{"testIdentifier": "T/testJourney", "attachments": [
            {"exportedFileName": "now.txt", "suggestedHumanReadableName": "App UI hierarchy_3.txt", "timestamp": 30},
            {"exportedFileName": "now.png", "suggestedHumanReadableName": "Screenshot_2.png", "timestamp": 20},
            {"exportedFileName": "old.png", "suggestedHumanReadableName": "Screenshot_1.png", "timestamp": 10},
            {"exportedFileName": "old.txt", "suggestedHumanReadableName": "App UI hierarchy_0.txt", "timestamp": 5}]}]
        self.assertEqual(journey.failure_attachments(manifest), ("now.png", "now.txt"))

    def test_a_runtime_warnings_earlier_capture_loses_to_the_failure_that_ended_the_test(self):
        manifest = [{"testIdentifier": "T/testJourney", "attachments": [
            {"exportedFileName": "fail.png", "suggestedHumanReadableName": "failure-screenshot_0_B.png", "timestamp": 303.8},
            {"exportedFileName": "fail.txt", "suggestedHumanReadableName": "failure-hierarchy_1_B.txt", "timestamp": 304.1},
            {"exportedFileName": "query.txt", "suggestedHumanReadableName": "Debug description for push-pre-prompt.txt",
             "timestamp": 303.6},
            {"exportedFileName": "warning.png", "suggestedHumanReadableName": "failure-screenshot_0_A.png", "timestamp": 216.8},
            {"exportedFileName": "warning.txt", "suggestedHumanReadableName": "failure-hierarchy_1_A.txt", "timestamp": 217.1}]}]
        self.assertEqual(journey.failure_attachments(manifest), ("fail.png", "fail.txt"))

    def test_the_assertion_message_is_the_first_error_line(self):
        log = "x.swift:12: error: -[UITests.A testJourney] : XCTAssertTrue failed - S2.3 missing\nother: error: -[B c] : later"
        self.assertEqual(journey.failure_message(log), "XCTAssertTrue failed - S2.3 missing")
        self.assertEqual(journey.failure_message("all good"), "")

    def test_a_failed_export_is_reported_and_does_not_raise(self):
        rows = [{"scenario": "S2", "result": "FAIL", "failed_step": "S2.3"}]
        with tempfile.TemporaryDirectory() as folder, redirect_stdout(StringIO()) as out, \
                unittest.mock.patch.object(journey, "sh", return_value=types.SimpleNamespace(returncode=1)):
            journey.export_evidence(Path(folder), rows, "")
        self.assertIn("failed at S2.3", out.getvalue())
        self.assertIn("could not export", out.getvalue())

POT_DOC = """---
id: cabals/pot
title: Pot
version: 1
requires: [auth/sign-in]
actors: [A]
flows: [01]
xcuitest: []
---

# Pot

## Scenarios

### S1 Look

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S1.1 | tap | `row-id` | | `pat-abc` shows |
"""

ROW_SWIFT = 'Text("x").accessibilityIdentifier("row-id")\n'
PATTERN_SWIFT = 'Text("y").accessibilityIdentifier(name)\n'
EVIDENCE = "governance/vote@7 commit %s runs 3 of 3 PASS seeded caught 4 of 4"


class Repo(Tree):
    """Two commits in a throwaway repo: a base, and a head that bumps sign-in to version 3 and seeds one bug."""

    def git(self, *args):
        done = subprocess.run(["git", "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"]
                              + list(args), cwd=str(journey.ROOT), check=True, stdout=subprocess.PIPE, universal_newlines=True)
        return done.stdout.strip()

    def commit(self, message):
        self.git("add", ".")
        self.git("commit", "-qm", message, "--no-verify")
        return self.git("rev-parse", "HEAD")

    def setUp(self):
        super().setUp()
        self.write("docs/journeys/cabals/pot.md", POT_DOC)
        self.write("apps/mobile/Monaco/Row.swift", ROW_SWIFT)
        self.write("apps/mobile/Monaco/Pattern.swift", PATTERN_SWIFT)
        self.write("qa/ids-composed.tsv", "pattern\tfile\twhy\npat-<x>\tapps/mobile/Monaco/Pattern.swift\tname\n")
        self.git("init", "-q")
        self.base = self.commit("base")
        self.write("docs/journeys/auth/sign-in.md", DOC.replace("version: 2 ", "version: 3 "))
        self.write("qa/auth/sign-in.mutants/a.patch", "expect-fail: S1\n")
        self.head = self.commit("head")

    def failures(self, body, fetch=lambda commit: False, head=None):
        return journey.gate_failures(self.base, head or self.head, body, fetch)

    def block(self, passed=3, total=3, caught=1, seeded=1, commit=None, version=3):
        line = "auth/sign-in@%d commit %s runs %d of %d PASS seeded caught %d of %d" % (
            version, commit or self.head, passed, total, caught, seeded)
        return "Proof\n\n```journey-evidence\n%s\n```\n" % line


class Gate(Repo):
    def test_a_version_bump_with_no_block_fails_with_the_command_to_run(self):
        self.assertEqual(self.failures("no proof here"), [
            "auth/sign-in@3: no evidence block; run scripts/qa/journey.py run auth/sign-in, "
            "then mutants, then report --evidence"])

    def test_a_block_for_another_version_or_journey_does_not_count(self):
        self.assertEqual(len(self.failures(self.block(version=2))), 1)
        self.assertEqual(len(self.failures(self.block().replace("auth/sign-in", "auth/other"))), 1)

    def test_a_valid_block_passes_and_lines_that_do_not_parse_are_ignored(self):
        self.assertEqual(self.failures(self.block()), [])
        self.assertEqual(self.failures("```journey-evidence\nnonsense\n```\n" + self.block()), [])

    def test_two_of_three_runs_fail(self):
        failures = self.failures(self.block(passed=2))
        self.assertEqual(len(failures), 1)
        self.assertTrue(failures[0].startswith("auth/sign-in@3: evidence shows 2 of 3 runs"))

    def test_one_passing_run_is_enough_and_one_failed_run_is_not(self):
        self.assertEqual(self.failures(self.block(passed=1, total=1)), [])
        failures = self.failures(self.block(passed=0, total=1))
        self.assertEqual(len(failures), 1)
        self.assertTrue(failures[0].startswith("auth/sign-in@3: evidence shows 0 of 1 runs"))

    def test_a_missed_seeded_bug_fails(self):
        failures = self.failures(self.block(caught=0))
        self.assertTrue(failures[0].startswith("auth/sign-in@3: evidence shows caught 0 of 1 seeded bugs"))

    def test_a_block_that_counts_fewer_seeded_bugs_than_the_journey_has_fails(self):
        self.assertIn("caught 0 of 0 seeded bugs, but the journey has 1", self.failures(self.block(caught=0, seeded=0))[0])

    def test_a_block_from_a_commit_with_other_journey_files_is_stale(self):
        failures = self.failures(self.block(commit=self.base))
        self.assertTrue(failures[0].startswith("auth/sign-in@3: evidence is from %s, whose journey files differ" % self.base[:12]))

    def test_a_commit_that_cannot_be_fetched_is_stale(self):
        asked = []
        failures = self.failures(self.block(commit="f" * 40), lambda commit: asked.append(commit) or False)
        self.assertEqual(asked, ["f" * 40])
        self.assertIn("whose journey files differ", failures[0])

    def test_a_block_from_an_earlier_commit_with_the_same_journey_files_passes(self):
        self.write("notes.txt", "later\n")
        later = self.commit("unrelated")
        self.assertEqual(journey.gate_failures(self.base, later, self.block(commit=self.head), lambda commit: False), [])

    def test_a_journey_the_pr_did_not_change_needs_no_block(self):
        self.write("apps/mobile/Monaco/Row.swift", ROW_SWIFT + "// edit\n")
        edit = self.commit("app edit")
        self.assertEqual(journey.gate_failures(self.head, edit, "", lambda commit: False), [])

    def test_a_changed_step_file_or_seeded_bug_needs_a_block(self):
        self.write("ui/SignInJourney.swift", JOURNEY_SWIFT + "// edit\n")
        edit = self.commit("steps")
        self.assertEqual(len(journey.gate_failures(self.head, edit, "", lambda commit: False)), 1)
        self.write("qa/auth/sign-in.mutants/b.patch", "expect-fail: S2\n")
        more = self.commit("bug")
        self.assertEqual(len(journey.gate_failures(edit, more, "", lambda commit: False)), 1)

    def test_a_new_journey_needs_a_block(self):
        self.assertEqual(len(journey.gate_failures(self.base, self.head, "")), 1)
        self.write("docs/journeys/cabals/new.md", POT_DOC.replace("cabals/pot", "cabals/new"))
        new = self.commit("new")
        self.assertIn("cabals/new@1: no evidence block", journey.gate_failures(self.head, new, self.block())[0])

    def test_a_fetched_commit_is_asked_for_once_and_the_files_then_compared(self):
        asked = []
        self.failures(self.block(commit="e" * 40), lambda commit: asked.append(commit) or True)
        self.assertEqual(asked, ["e" * 40])

    def test_the_gate_prints_failures_and_affected_warnings_and_exits_1(self):
        body = Path(self.tmp.name) / "body.md"
        body.write_text("nothing")
        args = types.SimpleNamespace(base=self.base, head=self.head, body_file=str(body))
        with redirect_stdout(StringIO()) as out:
            self.assertEqual(journey.cmd_gate(args), 1)
        self.assertIn("auth/sign-in@3: no evidence block", out.getvalue())
        self.assertIn("::warning::journey cabals/pot is affected by this change", out.getvalue())
        body.write_text(self.block())
        with redirect_stdout(StringIO()) as out:
            self.assertEqual(journey.cmd_gate(args), 0)
        self.assertNotIn("no evidence block", out.getvalue())

    def test_the_gate_reads_the_app_files_at_the_head_commit(self):
        self.write("apps/mobile/Monaco/Row.swift", 'Text("x").accessibilityIdentifier("row-id")\n')
        self.write("apps/mobile/Monaco/Row.swift", ROW_SWIFT + "// edit\n")
        edit = self.commit("app edit")
        body = Path(self.tmp.name) / "body.md"
        body.write_text("")
        with redirect_stdout(StringIO()) as out:
            self.assertEqual(journey.cmd_gate(types.SimpleNamespace(base=self.head, head=edit, body_file=str(body))), 0)
        self.assertIn("::warning::journey cabals/pot is affected", out.getvalue())

    def test_a_diff_with_no_merge_base_falls_back_to_the_two_commits(self):
        self.git("checkout", "-q", "--orphan", "other")
        self.git("rm", "-rfq", ".")
        self.write("lone.txt", "x\n")
        lone = self.commit("lone")
        self.assertIn("lone.txt", journey.changed_files(self.head, lone))

    def test_a_ref_that_does_not_exist_is_an_error(self):
        with self.assertRaises(journey.JourneyError):
            journey.changed_files("no-such-ref")


class Affected(Repo):
    def test_a_doc_change_lists_the_journey_and_every_journey_that_requires_it(self):
        self.assertEqual(journey.affected(["docs/journeys/auth/sign-in.md"], journey.load_journeys()),
                         ["auth/sign-in", "cabals/pot"])

    def test_the_requirers_of_a_requirer_are_listed_too(self):
        self.write("docs/journeys/cabals/deep.md", POT_DOC.replace("cabals/pot", "cabals/deep").replace(
            "requires: [auth/sign-in]", "requires: [cabals/pot]"))
        self.assertEqual(journey.affected(["docs/journeys/auth/sign-in.md"], journey.load_journeys()),
                         ["auth/sign-in", "cabals/deep", "cabals/pot"])

    def test_a_requirer_change_lists_only_itself(self):
        self.assertEqual(journey.affected(["qa/cabals/pot.truth.sh"], journey.load_journeys()), ["cabals/pot"])
        self.assertEqual(journey.affected(["qa/auth/sign-in.mutants/a.patch"], journey.load_journeys()),
                         ["auth/sign-in", "cabals/pot"])

    def test_step_files_and_setup_scripts_count(self):
        loaded = journey.load_journeys()
        self.assertEqual(journey.affected(["ui/SignInJourneyUITests.swift"], loaded), ["auth/sign-in", "cabals/pot"])
        self.assertEqual(journey.affected(["qa/auth/sign-in.setup.sh"], loaded), ["auth/sign-in", "cabals/pot"])

    def test_an_app_file_lists_the_journeys_whose_targets_it_sets(self):
        self.assertEqual(journey.affected(["apps/mobile/Monaco/Row.swift"], journey.load_journeys()), ["cabals/pot"])

    def test_a_composed_id_row_lists_the_journey_that_targets_the_pattern(self):
        self.assertEqual(journey.affected(["apps/mobile/Monaco/Pattern.swift"], journey.load_journeys()), ["cabals/pot"])

    def test_a_file_that_is_gone_or_not_swift_lists_nothing(self):
        loaded = journey.load_journeys()
        self.assertEqual(journey.affected(["apps/mobile/Monaco/Gone.swift", "README.md"], loaded), [])

    def test_the_command_prints_one_id_per_line_or_json(self):
        with redirect_stdout(StringIO()) as out:
            self.assertEqual(journey.main(["affected", "--base", self.base]), 0)
        self.assertEqual(out.getvalue().splitlines(), ["auth/sign-in", "cabals/pot"])
        with redirect_stdout(StringIO()) as out:
            journey.main(["affected", "--base", self.base, "--json"])
        self.assertEqual(json.loads(out.getvalue()), ["auth/sign-in", "cabals/pot"])


class EvidenceReport(Repo):
    def row(self, scenario="*", result="PASS", build=None, run="r1", version="3", expected="PASS", journey_id="auth/sign-in"):
        return {"journey": journey_id, "version": version, "scenario": scenario, "result": result, "run": run,
                "build": build or self.git("rev-parse", "--short", "HEAD"), "expected": expected}

    def mutant(self, result, run, build=None):
        build = (build or self.git("rev-parse", "--short", "HEAD")) + "+mutant:a"
        return [self.row("S1", result, build, run, expected="FAIL"), self.row("*", result, build, run, expected="FAIL")]

    def report(self, rows, ids=("auth/sign-in",)):
        return journey.evidence_report(journey.load_journeys(), rows, list(ids))

    def test_the_block_is_one_line_per_journey_in_the_format_the_gate_reads(self):
        rows = [self.row(run="r%d" % n) for n in range(4)] + self.mutant("FAIL", "m1")
        block = self.report(rows)
        self.assertEqual(block, "```journey-evidence\nauth/sign-in@3 commit %s runs 1 of 1 PASS seeded caught 1 of 1\n```" % self.head)
        self.assertEqual(self.failures(block), [])

    def test_an_older_failed_run_does_not_count(self):
        rows = [self.row(result="FAIL", run="old")] + [self.row(run="r%d" % n) for n in range(3)]
        self.assertIn("runs 1 of 1 PASS", self.report(rows + self.mutant("FAIL", "m")))

    def test_the_latest_clean_run_is_the_one_the_line_shows(self):
        failed_last = [self.row(run="a"), self.row(result="FAIL", run="b")]
        self.assertIn("runs 0 of 1 PASS", self.report(failed_last + self.mutant("FAIL", "m")))
        passed_last = [self.row(result="FAIL", run="a"), self.row(run="b")]
        self.assertIn("runs 1 of 1 PASS", self.report(passed_last + self.mutant("FAIL", "m")))


    def test_the_latest_result_of_each_seeded_bug_counts(self):
        clean = [self.row(run="r%d" % n) for n in range(3)]
        self.assertIn("caught 0 of 1", self.report(clean + self.mutant("FAIL", "m1") + self.mutant("PASS", "m2")))
        self.assertIn("caught 1 of 1", self.report(clean + self.mutant("PASS", "m1") + self.mutant("FAIL", "m2")))
        self.assertIn("caught 0 of 1", self.report(clean))

    def test_rows_of_another_version_or_commit_do_not_count(self):
        clean = [self.row(run="r%d" % n) for n in range(3)]
        stale = [self.row(version="2", run="v"), self.row(build="abcdef0", run="o"), self.row(build="dev", run="d")]
        self.assertIn("runs 1 of 1 PASS", self.report(stale + clean + self.mutant("FAIL", "m")))
        with self.assertRaises(journey.JourneyError) as raised:
            self.report(stale)
        self.assertIn("commit and rerun", str(raised.exception))

    def test_a_dirty_app_or_journey_file_is_an_error(self):
        rows = [self.row(run="r%d" % n) for n in range(3)]
        for name in ("apps/mobile/Monaco/Row.swift", "qa/auth/sign-in.mutants/a.patch", "ui/SignInJourney.swift"):
            self.write(name, "changed\n")
            with self.assertRaises(journey.JourneyError) as raised:
                self.report(rows)
            self.assertIn("uncommitted changes", str(raised.exception))
            self.git("checkout", "--", ".") if name != "ui/SignInJourney.swift" else self.git("checkout", "--", name)

    def test_the_report_command_prints_the_block_and_plain_report_is_unchanged(self):
        header = "\t".join(journey.COLUMNS) + "\n"
        with unittest.mock.patch.object(journey, "RESULTS", Path(self.tmp.name) / "results.tsv"):
            journey.RESULTS.write_text(header + "".join("\t".join(
                str(self.row(run="r%d" % n).get(c, "")) for c in journey.COLUMNS) + "\n" for n in range(3)))
            with redirect_stdout(StringIO()) as out:
                self.assertEqual(journey.main(["report", "--evidence", "auth/sign-in"]), 0)
            self.assertTrue(out.getvalue().startswith("```journey-evidence\nauth/sign-in@3 commit " + self.head))
            with redirect_stdout(StringIO()) as out:
                journey.main(["report"])
            self.assertTrue(out.getvalue().startswith("| Journey | Runs |"))


SHARDS = 6


def run_in_shards():
    """Runs the test classes in SHARDS processes; most tests wait on a subprocess, a server or a timeout."""
    suites = unittest.defaultTestLoader.loadTestsFromModule(sys.modules[__name__])
    classes = sorted({type(test).__name__ for suite in suites for test in suite})
    started = time.monotonic()
    procs = [subprocess.Popen([sys.executable, "-m", "unittest", "-q"] + ["test_journey." + name for name in classes[i::SHARDS]],
                              cwd=str(Path(__file__).resolve().parent), stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                              universal_newlines=True)
             for i in range(SHARDS)]
    failed = 0
    for proc in procs:
        out = proc.communicate()[0]
        if proc.returncode != 0:
            failed += 1
            sys.stdout.write(out)
    print("%d test classes in %d shards, %d failed, %.1f s" % (len(classes), SHARDS, failed, time.monotonic() - started))
    return 1 if failed else 0


if __name__ == "__main__":
    if len(sys.argv) > 1:
        unittest.main()
    sys.exit(run_in_shards())
