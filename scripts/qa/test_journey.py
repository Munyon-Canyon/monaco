#!/usr/bin/env python3
"""Tests for journey.py: the doc parser, the doc-against-test checks, and the log readers.

Run: python3 scripts/qa/test_journey.py
"""

import contextlib
import fcntl
import json
import os
import subprocess
import sys
import tempfile
import threading
import time
import unittest
import unittest.mock
from contextlib import redirect_stdout
from io import StringIO
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import journey  # noqa: E402

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
    func testS1SignIn() throws {}
    func testS2Relaunch() throws {}
}
"""


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

    def test_a_scenario_with_no_test_method_is_named(self):
        self.write("ui/SignInJourneyUITests.swift", TESTS_SWIFT.replace("    func testS2Relaunch() throws {}\n", ""))
        self.assertTrue(any("no test method named testS2" in p for p in self.problems()))

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


class Funds(Tree):
    def fund(self, lines):
        self.write("docs/journeys/auth/sign-in.md", DOC.replace("actors: [A]\n", "actors: [A]\nfunds:\n" + lines))

    def test_a_journey_without_funds_has_no_notice(self):
        self.assertEqual(journey.funding_notice(journey.load_journeys()["auth/sign-in"]), "")

    def test_a_money_journey_says_what_to_send_to_whom(self):
        self.fund("  A: 2\n")
        self.assertEqual(self.problems(), [])
        self.assertIn("send 2 USDC to actor A from the Phantom agent wallet",
                      journey.funding_notice(journey.load_journeys()["auth/sign-in"]))

    def test_funds_for_an_unknown_actor_or_a_bad_amount_are_named(self):
        self.fund("  B: 2\n  A: lots\n")
        problems = "\n".join(self.problems())
        self.assertIn("funds names actor B, which is not in actors", problems)
        self.assertIn("funds for actor A must be whole USDC from 1, got 'lots'", problems)

    def test_a_money_run_refuses_to_start_without_a_refund_address(self):
        with unittest.mock.patch.dict(os.environ, {"MONACO_QA_REFUND_ADDRESS": ""}):
            with self.assertRaisesRegex(journey.JourneyError, "MONACO_QA_REFUND_ADDRESS"):
                journey.require_refund_address()
        with unittest.mock.patch.dict(os.environ, {"MONACO_QA_REFUND_ADDRESS": "Phantom1"}):
            journey.require_refund_address()
            env = journey.actor_environment({}, "sms", "RUN123", prefix="TEST_RUNNER_")
        self.assertEqual(env["TEST_RUNNER_MONACO_QA_REFUND_ADDRESS"], "Phantom1")


class Composition(Tree):
    def add(self, journey_id, requires):
        self.write("docs/journeys/%s.md" % journey_id, DOC.replace("id: auth/sign-in", "id: " + journey_id)
                   .replace("requires: []", "requires: [%s]" % ", ".join(requires)))

    def test_a_cycle_is_named(self):
        self.add("cabal/create", ["cabal/join"])
        self.add("cabal/join", ["cabal/create"])
        self.assertTrue(any("requires form a cycle" in p for p in self.problems()))

    def test_phases_run_in_number_order_each_on_its_actor(self):
        self.write("ui/SignInJourneyUITests.swift", TESTS_SWIFT.replace(
            "    func testS2Relaunch() throws {}\n",
            "    func testS2Phase2BJoins() throws {}\n    func testS2Phase1ACreates() throws {}\n"
            "    func testS2Phase3ASeesB() throws {}\n"))
        loaded = journey.load_journeys()["auth/sign-in"]
        self.assertEqual(journey.xcuitest_phases(loaded, "S2"), [
            (1, "A", "SignInJourneyUITests/testS2Phase1ACreates"),
            (2, "B", "SignInJourneyUITests/testS2Phase2BJoins"),
            (3, "A", "SignInJourneyUITests/testS2Phase3ASeesB"),
        ])
        self.assertEqual(journey.xcuitest_phases(loaded, "S1"), [(1, "A", "SignInJourneyUITests/testS1SignIn")])

    def test_a_scenario_does_not_match_another_scenario_with_its_number_prefix(self):
        self.write("ui/SignInJourneyUITests.swift", """nonisolated final class SignInJourneyUITests: XCTestCase {
    func testS1A() throws {}
    func testS10B() throws {}
}
""")
        self.assertEqual(journey.xcuitest_phases(journey.load_journeys()["auth/sign-in"], "S1"), [
            (1, "A", "SignInJourneyUITests/testS1A"),
        ])


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
        threading.Timer(0.3, first.close).start()
        started = time.monotonic()
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
        saved = journey.listeners, journey.start_backend, journey.QA_LOCK
        folder = tempfile.TemporaryDirectory()
        self.addCleanup(folder.cleanup)

        def started(base_url, timeout=300):
            self.fail("started a backend while another one listens")

        journey.listeners = lambda: [("55430", "/w/644")]
        journey.start_backend = started
        self.addCleanup(lambda: setattr(journey, "listeners", saved[0]))
        self.addCleanup(lambda: setattr(journey, "start_backend", saved[1]))
        environ = dict(os.environ)
        environ.pop("MONACO_API_BASE_URL", None)
        with unittest.mock.patch.dict(os.environ, environ, clear=True), \
                unittest.mock.patch.object(journey, "take_qa_lock", lambda: open(Path(folder.name) / "l", "a")):
            with self.assertRaisesRegex(journey.JourneyError, "pid 55430 \\(/w/644\\)"):
                with journey.journey_backend():
                    self.fail("the run went ahead")


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


class Runner(Tree):
    def test_the_test_runner_passes_its_health_checked_api_url_to_the_app(self):
        loaded = journey.load_journeys()["auth/sign-in"]
        run_dir = journey.ROOT / "run"
        run_dir.mkdir()
        calls = []
        saved_sh = journey.sh

        def stub(args, **kwargs):
            calls.append(kwargs["env"])
            return type("Result", (), {"stdout": ""})()

        journey.sh = stub
        try:
            journey.run_xcuitest(loaded, ["S1"], {"A": "sim"}, journey.load_accounts(environ={}),
                                 "sms", run_dir, "http://127.0.0.1:8080", "RUN123")
        finally:
            journey.sh = saved_sh

        self.assertEqual(calls[0]["TEST_RUNNER_MONACO_QA_API_BASE_URL"], "http://127.0.0.1:8080")
        self.assertEqual(calls[0]["TEST_RUNNER_MONACO_QA_RUN"], "RUN123")

    def test_a_setup_script_runs_before_each_scenario_in_a_call_of_its_own(self):
        loaded = journey.load_journeys()["auth/sign-in"]
        self.write("qa/auth/sign-in.setup.sh", "#!/usr/bin/env bash\n")
        run_dir = journey.ROOT / "run"
        run_dir.mkdir()
        calls = []
        saved_sh = journey.sh

        def stub(args, **kwargs):
            if args[0].endswith("setup.sh"):
                calls.append(("setup", args[1], kwargs["env"]["MONACO_QA_RUN"], kwargs["env"]["MONACO_QA_HANDOFF"]))
                return type("Result", (), {"returncode": 0})()
            calls.append(("test", [a for a in args if a.startswith("-only-testing")]))
            return type("Result", (), {"stdout": ""})()

        journey.sh = stub
        try:
            journey.run_xcuitest(loaded, ["S1", "S2"], {"A": "sim"}, journey.load_accounts(environ={}),
                                 "sms", run_dir, "http://127.0.0.1:8080", "RUN123")
        finally:
            journey.sh = saved_sh

        self.assertEqual(calls, [
            ("setup", "S1", "RUN123", str(run_dir / "handoff.json")),
            ("test", ["-only-testing:MonacoUITests/SignInJourneyUITests/testS1SignIn"]),
            ("setup", "S2", "RUN123", str(run_dir / "handoff.json")),
            ("test", ["-only-testing:MonacoUITests/SignInJourneyUITests/testS2Relaunch"]),
        ])

    def test_a_failed_setup_stops_the_run_before_its_scenario(self):
        loaded = journey.load_journeys()["auth/sign-in"]
        self.write("qa/auth/sign-in.setup.sh", "#!/usr/bin/env bash\n")
        run_dir = journey.ROOT / "run"
        run_dir.mkdir()
        saved_sh = journey.sh
        journey.sh = lambda args, **kwargs: type("Result", (), {"returncode": 2, "stdout": ""})()
        try:
            with self.assertRaisesRegex(journey.JourneyError, "could not set up S1"):
                journey.run_xcuitest(loaded, ["S1"], {"A": "sim"}, journey.load_accounts(environ={}),
                                     "sms", run_dir, "http://127.0.0.1:8080", "RUN123")
        finally:
            journey.sh = saved_sh

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

    def test_an_unmapped_actor_gets_a_dedicated_simulator_built_like_gold(self):
        calls = []

        def stub(args, **kwargs):
            calls.append(args)
            if args == ["scripts/gold-sim-udid.sh"]:
                return self.result("gold\n")
            if args == ["xcrun", "simctl", "list", "devices", "--json"]:
                return self.result(json.dumps({"devices": self.devices}))
            if args[:3] == ["xcrun", "simctl", "create"]:
                return self.result("journey-a\n")
            self.fail("unexpected command: %r" % (args,))

        journey.sh = stub
        self.assertEqual(journey.resolve_simulators(self.loaded, {}), {"A": "journey-a"})
        self.assertEqual(calls[-1], [
            "xcrun", "simctl", "create", "Monaco Journeys A", "phone",
            "com.apple.CoreSimulator.SimRuntime.iOS-26-5",
        ])

    def test_a_lane_names_its_actor_simulators_after_itself(self):
        calls = []

        def stub(args, **kwargs):
            calls.append(args)
            if args == ["scripts/gold-sim-udid.sh"]:
                return self.result("gold\n")
            if args == ["xcrun", "simctl", "list", "devices", "--json"]:
                devices = dict(self.devices)
                devices["other"] = [{"udid": "primary-a", "name": "Monaco Journeys A", "isAvailable": True}]
                return self.result(json.dumps({"devices": devices}))
            if args[:3] == ["xcrun", "simctl", "create"]:
                return self.result("lane-a\n")
            if args[:2] == ["git", "rev-parse"]:
                return self.result(self.tmp.name + "\n")
            self.fail("unexpected command: %r" % (args,))

        journey.sh = stub
        journey.lane_name = lambda: "agent-7"
        self.assertEqual(journey.resolve_simulators(self.loaded, {}), {"A": "lane-a"})
        self.assertIn(["xcrun", "simctl", "create", "Monaco Journeys agent-7 A", "phone",
                       "com.apple.CoreSimulator.SimRuntime.iOS-26-5"], calls)
        registry = Path(self.tmp.name, "monaco-lane-sims.tsv").read_text()
        self.assertEqual(registry, "lane-a\tagent-7\tMonaco Journeys agent-7 A\n")

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

    def test_an_explicit_simulator_mapping_is_kept(self):
        def unexpected(*args, **kwargs):
            self.fail("simctl call: %r" % (args,))

        journey.sh = unexpected
        self.assertEqual(journey.resolve_simulators(self.loaded, {"A": "chosen"}), {"A": "chosen"})

    def test_a_conflicting_simulator_environment_stops_the_run(self):
        devices = {
            "runtime": [{"udid": "journey-a", "name": "Monaco Journeys A", "isAvailable": True}],
        }

        def stub(args, **kwargs):
            if args == ["xcrun", "simctl", "list", "devices", "--json"]:
                return self.result(json.dumps({"devices": devices}))
            if args == ["xcrun", "simctl", "boot", "journey-a"]:
                return self.result()
            if args == ["xcrun", "simctl", "bootstatus", "journey-a", "-b"]:
                return self.result()
            if args == ["xcrun", "simctl", "getenv", "journey-a", "MONACO_API_BASE_URL"]:
                return self.result("http://127.0.0.1:8082\n")
            self.fail("unexpected command: %r" % (args,))

        journey.sh = stub
        with self.assertRaisesRegex(journey.JourneyError, "simulator Monaco Journeys A has MONACO_API_BASE_URL=http://127.0.0.1:8082"):
            journey.check_simulator_api_environment({"A": "journey-a"}, "http://127.0.0.1:8080")

    def test_only_dedicated_simulators_are_reset_without_fresh(self):
        calls = []
        self.loaded.actors = ["A", "B"]
        devices = {
            "runtime": [
                {"udid": "journey-a", "name": "Monaco Journeys A", "isAvailable": True},
                {"udid": "chosen-b", "name": "Chosen B", "isAvailable": True},
            ],
        }

        def stub(args, **kwargs):
            calls.append(args)
            if args == ["xcrun", "simctl", "list", "devices", "--json"]:
                return self.result(json.dumps({"devices": devices}))
            return self.result()

        journey.sh = stub
        journey.reset_journey_simulators(self.loaded, {"A": "journey-a", "B": "chosen-b"}, {"B"}, False)
        self.assertEqual(calls[1:], [
            ["xcrun", "simctl", "boot", "journey-a"],
            ["xcrun", "simctl", "bootstatus", "journey-a", "-b"],
            ["xcrun", "simctl", "uninstall", "journey-a", "com.monaco.app"],
        ])

    def test_fresh_resets_an_explicit_simulator(self):
        calls = []

        def stub(args, **kwargs):
            calls.append(args)
            if args == ["xcrun", "simctl", "list", "devices", "--json"]:
                return self.result(json.dumps({"devices": self.devices}))
            return self.result()

        journey.sh = stub
        journey.reset_journey_simulators(self.loaded, {"A": "gold"}, {"A"}, True)
        self.assertEqual(calls[1:], [
            ["xcrun", "simctl", "boot", "gold"],
            ["xcrun", "simctl", "bootstatus", "gold", "-b"],
            ["xcrun", "simctl", "uninstall", "gold", "com.monaco.app"],
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


def _gone(probe, wait=5.0, every=0.05):
    """Polls probe until it raises ProcessLookupError; SIGKILL delivery and reaping lag the kill under load."""
    deadline = time.monotonic() + wait
    while True:
        try:
            probe()
        except ProcessLookupError:
            return True
        if time.monotonic() >= deadline:
            return False
        time.sleep(every)


class Budget(Output):
    def test_a_hung_test_call_is_killed_with_its_group_and_its_scenarios_time_out(self):
        pids = journey.ROOT / "pids"
        hang = self.write("hang.sh", "#!/bin/sh\necho $$ > %s\nsleep 60 &\necho $! >> %s\nwait\n" % (pids, pids))
        hang.chmod(0o755)
        args = ["run", "auth/sign-in", "--no-build", "--timeout", "2"]
        with unittest.mock.patch.object(journey, "journey_backend", lambda: _yielding("http://127.0.0.1:8080")), \
                unittest.mock.patch.object(journey, "resolve_simulators", lambda journey_, mapping: {"A": "sim"}), \
                unittest.mock.patch.object(journey, "check_simulator_api_environment", lambda sims, url: None), \
                unittest.mock.patch.object(journey, "reset_journey_simulators", lambda *a: None), \
                unittest.mock.patch.object(journey, "build_label", lambda mutant=None: "abc"), \
                unittest.mock.patch.object(journey, "xcodebuild", lambda sim, *extra: [str(hang)]), \
                redirect_stdout(StringIO()) as printed:
            started = time.monotonic()
            code = journey.main(args)
            took = time.monotonic() - started

        self.assertEqual(code, 1)
        self.assertLess(took, 10)
        self.assertIn("auth/sign-in timed out after 2 s in S1,S2 test", printed.getvalue())
        rows = [line.split("\t") for line in journey.RESULTS.read_text().splitlines()[1:]]
        results = {row[journey.COLUMNS.index("scenario")]: row[journey.COLUMNS.index("result")] for row in rows}
        self.assertEqual(results, {"S1": "TIMEOUT", "S2": "TIMEOUT", "*": "TIMEOUT"})
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
    """A full run whose xcodebuild prints a canned log: S1 fails at `s1_fails_at`, S2 passes."""

    def run_journey(self, s1_fails_at, known):
        self.write("docs/journeys/auth/sign-in.md", DOC + "\n## Known failures on staging\n\n" + known)
        test = "Test Case '-[MonacoUITests.SignInJourneyUITests %s]' %s"
        lines = [test % ("testS1SignIn", "started.")]
        for step in ("S1.1", "S1.2"):
            lines.append("JOURNEYSTEP\tbegin\t0\t%s" % step)
            if step == s1_fails_at:
                break
            lines.append("JOURNEYSTEP\tend\t0\t%s\t10" % step)
        lines.append(test % ("testS1SignIn", "failed (1.000 seconds)." if s1_fails_at else "passed (1.000 seconds)."))
        lines += [test % ("testS2Relaunch", "started."), "JOURNEYSTEP\tbegin\t0\tS2.1",
                  "JOURNEYSTEP\tend\t0\tS2.1\t10", test % ("testS2Relaunch", "passed (1.000 seconds).")]
        log = self.write("canned.log", "\n".join(lines) + "\n")
        fake = self.write("xcodebuild.sh", "#!/bin/sh\ncat %s\n" % log)
        fake.chmod(0o755)
        with unittest.mock.patch.object(journey, "journey_backend", lambda: _yielding("http://127.0.0.1:8080")), \
                unittest.mock.patch.object(journey, "resolve_simulators", lambda journey_, mapping: {"A": "sim"}), \
                unittest.mock.patch.object(journey, "check_simulator_api_environment", lambda sims, url: None), \
                unittest.mock.patch.object(journey, "reset_journey_simulators", lambda *a: None), \
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
        self.assertEqual(results, {"S1": ("KNOWN", "FAIL"), "S2": ("PASS", "PASS"), "*": ("PASS", "PASS")})
        self.assertIn("S1 KNOWN 1.0s at S1.2 (#2140)", printed)
        self.assertIn("| auth/sign-in | %s | S2 | S1 at S1.2 (#2140) | - | - |" % self.last_run(), self.report())

    def test_a_failure_at_an_unlisted_step_is_a_new_failure_and_the_run_fails(self):
        code, results, _ = self.run_journey("S1.1", "- S1.2: no form. Blocked by #2140.\n")
        self.assertEqual(code, 1)
        self.assertEqual(results, {"S1": ("FAIL", "PASS"), "S2": ("PASS", "PASS"), "*": ("FAIL", "PASS")})
        self.assertIn("| S2 | - | - | S1 FAIL at S1.1 |", self.report())

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

    def test_a_matching_stamp_and_test_bundle_skip_the_build(self):
        built = []
        with unittest.mock.patch.object(journey, "build", lambda sim, log, current=None: built.append(current)), \
                redirect_stdout(StringIO()) as printed:
            journey.ensure_build("sim", journey.OUT / "build.log")
            self.write("out/derived/build.stamp", journey.build_stamp() + "\n")
            self.write("out/derived/Build/Products/Monaco.xctestrun", "")
            journey.ensure_build("sim", journey.OUT / "build.log")
            journey.ensure_build("sim", journey.OUT / "build.log", rebuild=True)
        self.assertEqual(len(built), 2)
        self.assertIn("reusing build %s" % journey.build_stamp()[:8], printed.getvalue())


class All(Output):
    def test_every_journey_runs_after_what_it_requires_on_one_build_and_one_backend(self):
        self.write("docs/journeys/cabals/join.md", DOC.replace("auth/sign-in", "cabals/join").replace(
            "requires: []", "requires: [cabals/create]"))
        self.write("docs/journeys/cabals/create.md", DOC.replace("auth/sign-in", "cabals/create").replace(
            "requires: []", "requires: [auth/sign-in]"))
        self.write("docs/journeys/cabals/fund.md", DOC.replace("auth/sign-in", "cabals/fund").replace(
            "requires: []", "requires: []\nfunds:\n  A: 5"))
        backends, builds, ran = [], [], []

        @contextlib.contextmanager
        def backend():
            backends.append(1)
            yield "http://127.0.0.1:8080"

        def run_once(journey_, *a, **k):
            ran.append(journey_.id)
            return [], "PASS"

        with unittest.mock.patch.object(journey, "journey_backend", backend), \
                unittest.mock.patch.object(journey, "resolve_simulators", lambda journey_, mapping: {"A": "sim"}), \
                unittest.mock.patch.object(journey, "check_simulator_api_environment", lambda sims, url: None), \
                unittest.mock.patch.object(journey, "reset_journey_simulators", lambda *a: None), \
                unittest.mock.patch.object(journey, "ensure_build", lambda sim, log, rebuild=False: builds.append(sim)), \
                unittest.mock.patch.object(journey, "run_once", run_once), \
                unittest.mock.patch.object(journey, "build_label", lambda mutant=None: "abc"), \
                unittest.mock.patch.dict(os.environ, {"MONACO_QA_REFUND_ADDRESS": ""}), \
                redirect_stdout(StringIO()) as printed:
            code = journey.main(["run", "--all"])

        self.assertEqual(code, 0)
        self.assertEqual(ran, ["auth/sign-in", "cabals/create", "cabals/join"])
        self.assertEqual((len(backends), len(builds)), (1, 1))
        self.assertIn("cabals/fund SKIP funds", printed.getvalue())

    def test_run_needs_a_journey_or_all(self):
        with redirect_stdout(StringIO()), unittest.mock.patch("sys.stderr", StringIO()) as err:
            self.assertEqual(journey.main(["run"]), 2)
        self.assertIn("name one journey, or pass --all", err.getvalue())


class Seed(unittest.TestCase):
    """scripts/qa/seed.sh against a local server, with the token stubbed."""

    def setUp(self):
        import http.server

        class Handler(http.server.BaseHTTPRequestHandler):
            def do_GET(self):
                code, body = (200, b'{"id":"c1"}') if self.path == "/ok" else (404, b'{"error":"no cabal"}')
                self.send_response(code)
                self.end_headers()
                self.wfile.write(body)

            def log_message(self, *args):
                pass

        self.server = http.server.HTTPServer(("127.0.0.1", 0), Handler)
        threading.Thread(target=self.server.serve_forever, daemon=True).start()

    def tearDown(self):
        self.server.shutdown()
        self.server.server_close()

    def qa_api(self, path):
        script = 'source scripts/qa/seed.sh; qa_token() { echo tok; }; qa_api A GET %s' % path
        env = dict(os.environ, MONACO_API_BASE_URL="http://127.0.0.1:%d" % self.server.server_port)
        return subprocess.run(["bash", "-c", script], cwd=str(Path(__file__).resolve().parents[2]), env=env,
                              stdout=subprocess.PIPE, stderr=subprocess.PIPE, universal_newlines=True)

    def test_a_2xx_prints_the_body(self):
        done = self.qa_api("/ok")
        self.assertEqual((done.returncode, done.stdout), (0, '{"id":"c1"}'))

    def test_a_4xx_fails_and_prints_the_body(self):
        done = self.qa_api("/missing")
        self.assertNotEqual(done.returncode, 0)
        self.assertIn("HTTP 404", done.stderr)
        self.assertIn('{"error":"no cabal"}', done.stderr)


if __name__ == "__main__":
    unittest.main()
