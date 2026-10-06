#!/usr/bin/env python3
"""Tests for journey.py: the doc parser, the doc-against-test checks, and the log readers.

Run: python3 scripts/qa/test_journey.py
"""

import contextlib
import fcntl
import json
import os
import shutil
import signal
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

# No test takes a lock a real journey run on this Mac holds.
_LOCKS = tempfile.TemporaryDirectory()
journey.SLOT_LOCK = _LOCKS.name + "/slot%d.lock"
journey.ACTOR_LOCK = _LOCKS.name + "/actor-%s.lock"

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
        return unittest.mock.Mock(**{"all": False, "journey": "money/fund-cabal", **values})

    def pick(self, journey_id, environ):
        funded = unittest.mock.Mock(funds={"A": 2})
        free = unittest.mock.Mock(funds={})
        journeys = {"money/fund-cabal": funded, "auth/sign-in": free}
        with unittest.mock.patch.object(journey, "load_journeys", lambda: journeys), \
                unittest.mock.patch.dict(os.environ, environ, clear=True):
            return journey.backend_trade_engine(self.args(journey=journey_id))

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
                    unittest.mock.patch.object(journey, "apply_event_streams", lambda log: None), \
                    unittest.mock.patch.object(journey.subprocess, "Popen", popen), \
                    unittest.mock.patch.object(journey, "OUT", Path(tempfile.mkdtemp())):
                with self.assertRaises(RuntimeError):
                    journey.start_backend("http://127.0.0.1:1")
            self.assertEqual(seen["TRADE_ENGINE"], want)


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
        self.assertEqual(journey.SLOT_PORTS, ((8080, 8081), (8180, 8181)))

    def test_a_third_run_waits_for_a_slot(self):
        first, second = self.hold(journey.take_slot(), journey.take_slot())
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
        with self.assertRaisesRegex(journey.JourneyError, "--slot is 0 to 1"):
            journey.take_slot(2)

    def test_a_slot_starts_its_backend_on_its_own_ports(self):
        popen = []
        with unittest.mock.patch.object(journey.subprocess, "Popen", lambda *a, **k: popen.append(k["env"]) or
                                        type("P", (), {"poll": lambda self: None})()), \
                unittest.mock.patch.object(journey, "OUT", Path(tempfile.mkdtemp())), \
                unittest.mock.patch.object(journey, "apply_event_streams", lambda log: None), \
                unittest.mock.patch.object(journey, "backend_is_running", lambda url: True):
            journey.start_backend("http://127.0.0.1:8180", slot=1)
        self.assertEqual(popen[0]["MONACO_HTTP_ADDR"], ":8180")
        self.assertEqual(popen[0]["MONACO_WORKER_HEALTH_ADDR"], ":8181")
        self.assertRegex(popen[0]["MONACO_LOG_DIR"], r"backend-slot1-\d{8}T\d{6}Z$")

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
            ["xcrun", "simctl", "uninstall", "journey", "com.monaco.app"],
        ])

    def test_fresh_resets_an_explicit_simulator(self):
        calls = []
        journey.sh = lambda args, **kwargs: calls.append(args) or self.result()
        journey.reset_journey_simulator("gold", True, True)
        self.assertEqual(calls, [
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


class RemappedRun(Output):
    """A doc-actor-A journey that holds login C must not touch A's simulator or derived data."""

    def accounts(self):
        row = {"actor": "A", "phone": "3", "email": "", "code": "", "name": "", "login": "C"}
        return {"A": row}

    def derived_arg(self):
        args = journey.xcodebuild("sim")
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
                unittest.mock.patch.object(journey, "apply_event_streams", lambda log: logs.append(log.name)), \
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
        self.assertLess(order.index("docker compose up -d --wait postgres nats"), len(order) - 2)

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
        with unittest.mock.patch.object(journey, "journey_backend", lambda *a: _yielding("http://127.0.0.1:8080")), \
                unittest.mock.patch.object(journey, "resolve_simulator", lambda override="", logins=None: "sim"), \
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
                unittest.mock.patch.object(journey, "resolve_simulator", lambda override="", logins=None: "sim"), \
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
                unittest.mock.patch.object(journey, "resolve_simulator", lambda override="", logins=None: "sim"), \
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
        def backend(*a):
            backends.append(1)
            yield "http://127.0.0.1:8080"

        def run_once(journey_, *a, **k):
            ran.append(journey_.id)
            return [], "PASS"

        with unittest.mock.patch.object(journey, "journey_backend", backend), \
                unittest.mock.patch.object(journey, "resolve_simulator", lambda override="", logins=None: "sim"), \
                unittest.mock.patch.object(journey, "check_simulator_api_environment", lambda sim, url: None), \
                unittest.mock.patch.object(journey, "prepare_logins", lambda *a: None), \
                unittest.mock.patch.object(journey, "reset_journey_simulator", lambda *a: None), \
                unittest.mock.patch.object(journey, "ensure_build", lambda sim, log, rebuild=False: builds.append(sim)), \
                unittest.mock.patch.object(journey, "run_once", run_once), \
                unittest.mock.patch.object(journey, "build_label", lambda mutant=None: "abc"), \
                unittest.mock.patch.dict(os.environ, {"MONACO_QA_REFUND_ADDRESS": ""}), \
                unittest.mock.patch.object(journey, "with_dotenv", lambda args: ""), \
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
        script = 'source scripts/qa/seed.sh; qa_token() { echo tok; }; qa_api A GET %s' % path
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
                unittest.mock.patch.object(journey, "apply_event_streams", lambda log: None), \
                unittest.mock.patch.object(journey.subprocess, "Popen", popen), redirect_stdout(StringIO()):
            with self.assertRaises(StopIteration):
                journey.start_backend("http://127.0.0.1:8080")
        return started[0]

    def test_fake_rpc_points_the_backend_at_the_fakes_server(self):
        env = self.env_of_started_backend({"QA_FAKE_RPC": "1", "SOLANA_RPC_URL": "https://mainnet"})
        self.assertEqual(env["SOLANA_RPC_URL"], "http://127.0.0.1:8099/rpc/")
        env = self.env_of_started_backend({"QA_FAKE_RPC": "1", "QA_FAKES_URL": "http://127.0.0.1:9000"})
        self.assertEqual(env["SOLANA_RPC_URL"], "http://127.0.0.1:9000/rpc/")

    def test_without_fake_rpc_the_backend_keeps_its_rpc(self):
        env = self.env_of_started_backend({"SOLANA_RPC_URL": "https://mainnet"})
        self.assertEqual(env["SOLANA_RPC_URL"], "https://mainnet")


if __name__ == "__main__":
    unittest.main()
