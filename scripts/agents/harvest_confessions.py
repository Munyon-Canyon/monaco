#!/usr/bin/env python3
"""Harvest the "What came up" and "Reviewer focus" bullets of landed PRs and count them by category."""

from __future__ import annotations

import argparse
import json
import os
import pathlib
import re
import shutil
import subprocess
import sys

CATEGORIES: list[tuple[str, tuple[str, ...]]] = [
    (
        "edits outside Touches",
        ("touches", "outside the ticket", "outside the listed", "not in the ticket", "off a touches", "scope creep"),
    ),
    (
        "gate failed on first push",
        ("ratchet", "first push", "pre-commit", "golangci", "lint failed", "lint finding", "gate failed",
         "failed on the first", "stage 0 failed", "coverage gate", "mutation", "gremlins", "format check failed"),
    ),
    (
        "test edited to fit",
        ("golden", "edited the test", "changed the test", "loosened", "adjusted the assertion", "updated the expected",
         "rewrote the test", "test expected", "now expects", "old assertion", "no longer matches", "assertion"),
    ),
    (
        "verifier caught after first push",
        ("verifier caught", "verifier found", "verifier flagged", "verifier refused", "verdict refused"),
    ),
    (
        "deferred to the operator",
        ("operator", "needs from logan", "logan must", "logan to", "left for", "human decision", "human only"),
    ),
    (
        "verification skipped",
        ("not run", "did not run", "didn't run", "not verified", "unverified", "skipped", "could not run",
         "couldn't run", "no simulator", "not exercised", "untested", "not tested"),
    ),
    (
        "flaky or slow CI",
        ("flake", "flaky", "rerun", "timed out", "timeout", "slow ci", "queue wait", "retry the job", "runner"),
    ),
    (
        "concurrency and idempotency",
        ("race", "idempot", "concurren", "deadlock", "goroutine", "advisory lock", "double-", "twice", "replay",
         "redeliver", "at-least-once", "mutex"),
    ),
    (
        "contract, golden and event shape",
        ("openapi", "contract", "event subject", "payload", "schema", "generated", "sqlc", "migration", "json shape",
         "wire format", "problem+json"),
    ),
    (
        "money and price correctness",
        ("price", "usdc", "slippage", "floor", "fee", "bps", "amount", "decimals", "rounding", "nav", "balance",
         "quote", "swap", "ledger", "share", "lamport", "wei", "fx rate"),
    ),
    (
        "env and tooling",
        ("env", "dotenvx", "docker", "toolchain", "xcode", "simulator", "gt ", "graphite", "worktree", "homebrew",
         "go version", "dependency", "cache", "hook", "guard"),
    ),
    (
        "stale ticket spec",
        ("stale", "ticket said", "ticket asked", "spec said", "ticket spec", "ticket named", "no longer exists",
         "already exists", "already done", "the ticket", "the issue", "ticket does not", "ticket also", "ticket is wrong", "ticket was wrong"),
    ),
    (
        "process",
        ("stack", "restack", "land-stack", "pr body", "split", "branch", "conflict", "rebase", "queue", "parent"),
    ),
]
FALLBACK = "other"
KINDS = {"what came up": "came_up", "reviewer focus": "reviewer_focus"}
BULLET = re.compile(r"^(?:[-*+]|\d+[.)])\s+(.*)$")
FENCE = re.compile(r"^\s*(```|~~~)")


def parse_items(body: str) -> list[tuple[str, str]]:
    """Return (kind, text) for each top-level bullet under the two harvested sections, in body order."""
    items: list[tuple[str, str]] = []
    kind: str | None = None
    current: list[str] | None = None
    in_fence = False

    def flush() -> None:
        nonlocal current
        if current is not None and kind is not None:
            text = " ".join(" ".join(current).split())
            if text:
                items.append((kind, text))
        current = None

    for line in (body or "").replace("\r\n", "\n").split("\n"):
        if FENCE.match(line):
            in_fence = not in_fence
        heading = None if in_fence else re.match(r"^##\s+(.*?)\s*$", line)
        if heading:
            flush()
            kind = KINDS.get(heading.group(1).lower())
            continue
        if kind is None:
            continue
        bullet = None if in_fence else BULLET.match(line)
        if bullet:
            flush()
            current = [bullet.group(1)]
        elif current is not None and (in_fence or line.strip()):
            current.append(line.strip())
        elif current is not None and not line.strip():
            flush()
    flush()
    return items


def classify(text: str) -> str:
    """Pick the category with the most keyword hits; ties go to the earlier table row."""
    low = text.lower()
    best, best_hits = FALLBACK, 0
    for name, words in CATEGORIES:
        hits = sum(len(re.findall(r"\b" + re.escape(w), low)) for w in words)
        if hits > best_hits:
            best, best_hits = name, hits
    return best


def have_gh_auth() -> bool:
    if os.environ.get("GH_TOKEN"):
        return True
    if shutil.which("gh") is None:
        return False
    return subprocess.run(["gh", "auth", "status"], capture_output=True).returncode == 0


def list_prs(limit: int, since: int | None, verbose: bool) -> list[dict]:
    cmd = [
        "gh", "pr", "list", "--state", "closed", "--base", "staging", "--limit", str(limit),
        "--json", "number,title,body,additions,deletions,closedAt",
    ]
    if verbose:
        print("+ " + " ".join(cmd), file=sys.stderr)
    out = subprocess.run(cmd, capture_output=True, text=True)
    if out.returncode != 0:
        sys.exit(f"gh pr list failed: {out.stderr.strip()}")
    prs = json.loads(out.stdout)
    prs = [p for p in prs if since is None or p["number"] >= since]
    return sorted(prs, key=lambda p: -p["number"])


def collect(prs: list[dict]) -> list[dict]:
    rows = []
    for pr in prs:
        for kind, text in parse_items(pr.get("body") or ""):
            rows.append({"pr": pr["number"], "kind": kind, "category": classify(text), "text": text})
    return rows


def render_table(rows: list[dict]) -> str:
    counts: dict[str, list] = {}
    for r in rows:
        entry = counts.setdefault(r["category"], [0, set()])
        entry[0] += 1
        entry[1].add(r["pr"])
    ordered = sorted(counts.items(), key=lambda kv: (-kv[1][0], kv[0]))
    lines = ["| Category | Items | PRs |", "| --- | ---: | ---: |"]
    lines += [f"| {name} | {n} | {len(prs)} |" for name, (n, prs) in ordered]
    all_prs = {r["pr"] for r in rows}
    lines.append(f"| Total | {len(rows)} | {len(all_prs)} |")
    return "\n".join(lines)


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(
        description="Count the What came up and Reviewer focus bullets of landed PRs into staging by category.",
    )
    ap.add_argument("--limit", type=int, default=400, help="how many closed PRs to list (default 400)")
    ap.add_argument("--since", type=int, default=None, help="lowest PR number to include")
    ap.add_argument("--out", default=".build/harvest", help="directory for items.jsonl (default .build/harvest)")
    ap.add_argument("--verbose", action="store_true", help="print the gh call and per-run counts to stderr")
    args = ap.parse_args(argv)

    if not have_gh_auth():
        print("gh is not authenticated: run `gh auth login` or set GH_TOKEN", file=sys.stderr)
        return 2
    prs = list_prs(args.limit, args.since, args.verbose)
    rows = collect(prs)
    out = pathlib.Path(args.out)
    out.mkdir(parents=True, exist_ok=True)
    with open(out / "items.jsonl", "w", encoding="utf-8", newline="\n") as f:
        for r in rows:
            f.write(json.dumps(r, sort_keys=True, ensure_ascii=False) + "\n")
    if args.verbose:
        print(f"{len(prs)} PRs, {len(rows)} items -> {out / 'items.jsonl'}", file=sys.stderr)
    print(render_table(rows))
    return 0


if __name__ == "__main__":
    sys.exit(main())
