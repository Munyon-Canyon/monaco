#!/usr/bin/env python3
"""A subagent that reports done can leave `gh run watch`, `go test` or `tail -f`
running under the Claude Code process.
"""

from __future__ import annotations

import glob
import json
import os
import signal
import subprocess
import sys


def own_shells(event: dict) -> list[dict]:
    try:
        with open(event.get("agent_transcript_path") or "") as f:
            transcript = f.read()
    except OSError:
        return []
    return [
        t for t in event.get("background_tasks") or []
        if t.get("type") == "shell" and t.get("status") == "running"
        and t.get("id") and t.get("command") and t["id"] in transcript
    ]


def processes() -> list[tuple[int, int, int, str]]:
    out = subprocess.run(["ps", "-ww", "-A", "-o", "pid=,ppid=,pgid=,command="],
                         capture_output=True, text=True).stdout
    rows = []
    for line in out.splitlines():
        parts = line.split(None, 3)
        if len(parts) == 4 and parts[0].isdigit():
            rows.append((int(parts[0]), int(parts[1]), int(parts[2]), parts[3]))
    return rows


def quoted(command: str) -> str:
    return "'" + command.replace("'", "'\"'\"'") + "'"


def reap(shells: list[dict], claude_pid: int) -> list[str]:
    protected = {os.getpgrp(), os.getpgid(claude_pid) if pid_alive(claude_pid) else -1}
    killed = []
    for pid, ppid, pgid, cmdline in processes():
        if ppid != claude_pid:
            continue
        task = next((t for t in shells if quoted(t["command"]) in cmdline or cmdline.endswith(t["command"])), None)
        if not task:
            continue
        try:
            if pgid == pid and pgid not in protected:
                os.killpg(pgid, signal.SIGTERM)
            else:
                os.kill(pid, signal.SIGTERM)
        except OSError:
            continue
        killed.append(f"{task['id']} ({task['command']})")
    return killed


def pid_alive(pid: int) -> bool:
    try:
        os.kill(pid, 0)
    except ProcessLookupError:
        return False
    except PermissionError:
        return True
    return True


def queue_dirs() -> list[str]:
    override = os.environ.get("HEAVY_QUEUE_GLOB")
    patterns = [override] if override else [f"{root}/claude-{os.getuid()}/*heavy.queue" for root in ("/tmp", "/private/tmp")]
    return sorted({os.path.realpath(d) for p in patterns for d in glob.glob(p) if os.path.isdir(d)})


def drop_stale_tickets() -> list[str]:
    dropped = []
    for d in queue_dirs():
        for ticket in os.listdir(d):
            pid = ticket.rsplit("-", 1)[-1]
            if pid.isdigit() and not pid_alive(int(pid)):
                try:
                    os.remove(os.path.join(d, ticket))
                    dropped.append(os.path.join(d, ticket))
                except OSError:
                    pass
    return dropped


def main() -> int:
    try:
        event = json.load(sys.stdin)
    except ValueError:
        return 0
    claude_pid = int(os.environ.get("CLAUDE_PID") or os.getppid())
    killed = reap(own_shells(event), claude_pid)
    dropped = drop_stale_tickets()
    for k in killed:
        print(f"agent-reap: stopped background shell {k}")
    for d in dropped:
        print(f"agent-reap: removed stale queue ticket {d}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
