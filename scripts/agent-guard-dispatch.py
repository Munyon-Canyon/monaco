#!/usr/bin/env python3
"""PreToolUse hook on the Agent tool. Exit 2 blocks the spawn and shows stderr to the agent."""

from __future__ import annotations

import json
import os
import re
import subprocess
import sys

AGENT_TYPE = "pstack:poteto-agent"
MODELS = ("opus", "sonnet")
BRIEF_RE = re.compile(r"^[\s>*-]*brief\s*:[\s*]*`?\S*?docs/agents/(owner|verifier)\.md`?[\s*]*$", re.M | re.I)
TICKET_RE = re.compile(r"^\s*ticket:\s*#?(\d+)\s*$", re.M)


def records_dir(cwd: str) -> str | None:
    try:
        out = subprocess.run(["git", "rev-parse", "--path-format=absolute", "--git-common-dir"],
                             cwd=cwd, capture_output=True, text=True, timeout=10)
    except (OSError, subprocess.SubprocessError):
        return None
    if out.returncode != 0:
        return None
    return os.path.join(out.stdout.strip(), ".monaco", "agents")


def verdict(tool_input: dict, cwd: str) -> str | None:
    prompt = tool_input.get("prompt") or ""
    brief = BRIEF_RE.search(prompt)
    if not brief:
        return None
    role = brief.group(1)
    kind = tool_input.get("subagent_type") or "general-purpose"
    if kind != AGENT_TYPE:
        return (f"{role} spawns run as subagent_type {AGENT_TYPE}, not {kind}. "
                "Pass the spawn line that `monacoctl agents dispatch` or `verify-plan` printed.")
    model = tool_input.get("model")
    if model not in MODELS:
        got = model or "none (inherits yours)"
        return (f"{role} spawns need an explicit model, one of {', '.join(MODELS)}; got {got}. "
                "Owners take the model given to dispatch; verifiers take the one `monacoctl agents verify-plan` prints.")
    if role != "owner":
        return None
    ticket = TICKET_RE.search(prompt)
    if not ticket:
        return None
    records = records_dir(cwd)
    if records is None:
        return f"could not find the git common dir from {cwd} to read the dispatch record for #{ticket.group(1)}."
    if not os.path.isfile(os.path.join(records, f"{ticket.group(1)}.json")):
        return (f"#{ticket.group(1)} has no dispatch record. Run `monacoctl agents dispatch {ticket.group(1)} "
                "--model <m>` first: it refuses a ticket whose blockers have not merged, then prints this spawn.")
    return None


def main() -> int:
    try:
        event = json.load(sys.stdin)
    except ValueError:
        return 0
    if event.get("tool_name") not in {"Agent", "Task"}:
        return 0
    reason = verdict(event.get("tool_input") or {}, event.get("cwd") or os.getcwd())
    if reason:
        print(f"blocked by scripts/agent-guard-dispatch.py: {reason}", file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    sys.exit(main())
