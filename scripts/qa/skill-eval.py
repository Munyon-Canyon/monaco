#!/usr/bin/env python3
"""Check that a skill is picked up when it should be, and left alone when it should not.

  skill-eval.py <skill dir> [--trials N] [--only ID] [--timeout S] [--agent CMD]

Reads <skill dir>/evals/cases.json: a list of {id, prompt, should_trigger}. Each case runs
in a fresh headless agent that can read the repo and load skills but cannot change anything,
so a case leaves no trace for the next one. A run counts as a trigger when the agent loads
the skill or reads its SKILL.md. Agents are not deterministic: a case passes only when every
trial agrees with should_trigger. Exits 1 when any case fails.
"""

import argparse
from collections import Counter
import json
import os
import shlex
import subprocess
import sys
import threading
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
DEFAULT_AGENT = "claude -p --output-format stream-json --verbose --no-session-persistence --tools Skill Read Grep Glob --"


def triggered_by(event, skill):
    """True when one stream-json event shows the agent reaching for the skill."""
    if event.get("type") != "assistant":
        return False
    for block in (event.get("message") or {}).get("content") or []:
        if not isinstance(block, dict) or block.get("type") != "tool_use":
            continue
        tool_input = block.get("input") or {}
        if block.get("name") == "Skill" and str(tool_input.get("skill", "")).split(":")[-1] == skill:
            return True
        if block.get("name") == "Read" and ("skills/%s/" % skill) in str(tool_input.get("file_path", "")):
            return True
    return False


def run_case(agent, prompt, skill, timeout):
    """(triggered, how the run ended). Stops the agent at the first trigger: that is the answer."""
    process = subprocess.Popen(shlex.split(agent) + [prompt], cwd=str(ROOT), stdout=subprocess.PIPE,
                               stderr=subprocess.PIPE, universal_newlines=True)
    timer_fired = threading.Event()

    def expire():
        timer_fired.set()
        process.kill()

    timer = threading.Timer(timeout, expire)
    timer.start()
    triggered, ended = False, None
    try:
        for line in process.stdout:
            try:
                event = json.loads(line)
            except ValueError:
                continue
            if triggered_by(event, skill):
                triggered, ended = True, "triggered"
                process.kill()
                break
            if event.get("type") == "result":
                ended = "error" if event.get("is_error") else "finished"
                break
    finally:
        timer.cancel()
        return_code = process.wait()
        stderr = process.stderr.read()
        process.stdout.close()
        process.stderr.close()
    if timer_fired.is_set():
        ended = "timeout"
    elif ended is None:
        ended = "error" if return_code else "exited"
    return triggered, ended, stderr


def verdict(should_trigger, outcomes):
    """PASS when every trial agrees with should_trigger and finishes successfully."""
    return "PASS" if all(
        triggered == should_trigger and ended not in ("exited", "error", "timeout")
        for triggered, ended, _ in outcomes
    ) else "FAIL"


def endings_summary(outcomes):
    counts = Counter(ended for _, ended, _ in outcomes)
    return ", ".join("%s x%d" % (ended, counts[ended]) for ended in sorted(counts))


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("skill_dir")
    parser.add_argument("--trials", type=int, default=3)
    parser.add_argument("--only", action="append", metavar="ID")
    parser.add_argument("--timeout", type=int, default=180, help="seconds one trial may take")
    parser.add_argument("--agent", default=os.environ.get("MONACO_SKILL_EVAL_AGENT", DEFAULT_AGENT),
                        help="the agent command; the prompt is added as its last argument and it must print stream-json")
    args = parser.parse_args(argv)

    suite = json.loads((Path(args.skill_dir) / "evals" / "cases.json").read_text())
    cases = [case for case in suite["cases"] if not args.only or case["id"] in args.only]
    failed = 0
    print("| Case | Should trigger | Triggered | Endings | Result |")
    print("| --- | --- | --- | --- | --- |")
    for case in cases:
        outcomes = [run_case(args.agent, case["prompt"], suite["skill"], args.timeout) for _ in range(args.trials)]
        result = verdict(case["should_trigger"], outcomes)
        failed += result == "FAIL"
        hits = sum(1 for triggered, _, _ in outcomes if triggered)
        print("| %s | %s | %d of %d | %s | %s |" % (
            case["id"], "yes" if case["should_trigger"] else "no", hits, args.trials,
            endings_summary(outcomes), result,
        ))
        for _, ended, stderr in outcomes:
            if ended == "error":
                lines = stderr.rstrip().splitlines()
                if lines:
                    print("  stderr: %s" % lines[-1])
        sys.stdout.flush()
    print("\n%d of %d cases passed" % (len(cases) - failed, len(cases)))
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
