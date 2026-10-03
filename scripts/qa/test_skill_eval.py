#!/usr/bin/env python3
"""Tests for skill-eval.py. Run: python3 scripts/qa/test_skill_eval.py"""

import importlib.util
import json
import pathlib
import shlex
import sys
import unittest

spec = importlib.util.spec_from_file_location("skill_eval", pathlib.Path(__file__).with_name("skill-eval.py"))
skill_eval = importlib.util.module_from_spec(spec)
spec.loader.exec_module(skill_eval)


def tool_use(name, tool_input):
    content = [{"type": "text", "text": "thinking"}, {"type": "tool_use", "name": name, "input": tool_input}]
    return {"type": "assistant", "message": {"content": content}}


class Trigger(unittest.TestCase):
    def test_loading_the_skill_is_a_trigger(self):
        self.assertTrue(skill_eval.triggered_by(tool_use("Skill", {"skill": "ios-journey-qa"}), "ios-journey-qa"))

    def test_a_plugin_prefixed_name_is_the_same_skill(self):
        self.assertTrue(skill_eval.triggered_by(tool_use("Skill", {"skill": "monaco:ios-journey-qa"}), "ios-journey-qa"))

    def test_reading_the_skill_file_is_a_trigger(self):
        event = tool_use("Read", {"file_path": "/repo/.claude/skills/ios-journey-qa/SKILL.md"})
        self.assertTrue(skill_eval.triggered_by(event, "ios-journey-qa"))

    def test_another_skill_or_another_event_is_not(self):
        self.assertFalse(skill_eval.triggered_by(tool_use("Skill", {"skill": "ios-verify"}), "ios-journey-qa"))
        self.assertFalse(skill_eval.triggered_by(tool_use("Read", {"file_path": "/repo/docs/journeys/README.md"}), "ios-journey-qa"))
        self.assertFalse(skill_eval.triggered_by({"type": "result", "result": "ios-journey-qa"}, "ios-journey-qa"))


class Verdict(unittest.TestCase):
    def test_every_trial_must_agree(self):
        self.assertEqual(skill_eval.verdict(True, [(True, "triggered", ""), (True, "triggered", "")]), "PASS")
        self.assertEqual(skill_eval.verdict(True, [(True, "triggered", ""), (False, "finished", "")]), "FAIL")
        self.assertEqual(skill_eval.verdict(False, [(False, "finished", ""), (True, "triggered", "")]), "FAIL")

    def test_an_unsuccessful_trial_fails_regardless_of_trigger_expectation(self):
        self.assertEqual(skill_eval.verdict(False, [(False, "timeout", "")]), "FAIL")
        self.assertEqual(skill_eval.verdict(False, [(False, "exited", "")]), "FAIL")
        self.assertEqual(skill_eval.verdict(False, [(False, "error", "")]), "FAIL")


class RunCase(unittest.TestCase):
    def agent(self, code):
        return "%s -c %s" % (shlex.quote(sys.executable), shlex.quote(code))

    def test_nonzero_exit_without_output_is_error_and_fails_a_negative_case(self):
        outcome = skill_eval.run_case(self.agent("import sys; sys.exit(1)"), "prompt", "ios-journey-qa", 5)
        self.assertEqual(outcome[1], "error")
        self.assertEqual(skill_eval.verdict(False, [outcome]), "FAIL")

    def test_error_result_event_ends_as_error(self):
        event = json.dumps({"type": "result", "is_error": True})
        outcome = skill_eval.run_case(self.agent("print(%r)" % event), "prompt", "ios-journey-qa", 5)
        self.assertEqual(outcome[1], "error")

    def test_normal_result_without_a_trigger_passes_a_negative_case(self):
        event = json.dumps({"type": "result", "is_error": False})
        outcome = skill_eval.run_case(self.agent("print(%r)" % event), "prompt", "ios-journey-qa", 5)
        self.assertEqual(outcome[1], "finished")
        self.assertEqual(skill_eval.verdict(False, [outcome]), "PASS")


if __name__ == "__main__":
    unittest.main()
