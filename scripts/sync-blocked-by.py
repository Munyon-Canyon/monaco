#!/usr/bin/env python3
"""Mirror each open ticket's "Blocked by" header into GitHub's native blocked-by links.

The header line is the source of truth; the links only drive GitHub's UI.
Dry run by default. Usage:

    scripts/sync-blocked-by.py --milestone "M7 — Backend platform" --tracking 492 [--repo owner/name] [--apply]
"""
from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys

HEADER_RE = re.compile(r"\*\*Blocked by:\*\*\s*([^·\n]*)")


class GhError(Exception):
    pass


def gh(*args: str) -> str:
    proc = subprocess.run(["gh", *args], capture_output=True, text=True)
    if proc.returncode != 0:
        raise GhError(f"gh {' '.join(args)}: {proc.stderr.strip() or proc.stdout.strip()}")
    return proc.stdout


def parse_pages(raw: str) -> list:
    """Read every JSON value gh --paginate concatenated onto stdout."""
    decoder = json.JSONDecoder()
    items: list = []
    idx = 0
    length = len(raw)
    while True:
        while idx < length and raw[idx].isspace():
            idx += 1
        if idx >= length:
            return items
        value, idx = decoder.raw_decode(raw, idx)
        if not isinstance(value, list):
            raise GhError(f"blocked_by page is {type(value).__name__}, want a JSON array")
        items.extend(value)


def already_taken(err: GhError) -> bool:
    text = str(err)
    return "422" in text and "already been taken" in text


def blocked_by(api: str, number: int) -> list:
    raw = gh("api", "--paginate", f"{api}/issues/{number}/dependencies/blocked_by?per_page=100")
    return parse_pages(raw)


def wanted(body: str | None) -> set[int] | None:
    lines = (body or "").splitlines()
    first = lines[0] if lines else ""
    m = HEADER_RE.search(first)
    if not m:
        return None
    return {int(n) for n in re.findall(r"#(\d+)", m.group(1))}


def sync(repo: str, milestone: str, tracking: int, apply: bool, out) -> None:
    api = f"repos/{repo}"
    issues = json.loads(gh("issue", "list", "--repo", repo, "--milestone", milestone, "--state", "open",
                           "--limit", "500", "--json", "number,body"))
    for issue in sorted(issues, key=lambda i: i["number"]):
        n = issue["number"]
        if n == tracking:
            continue
        have = {d["number"]: d["id"] for d in blocked_by(api, n)}
        want = wanted(issue["body"])
        if want is None:
            print(f"#{n}: no Blocked by header; links {sorted(have)}", file=out)
            continue
        add, remove = sorted(want - set(have)), sorted(set(have) - want)
        if not add and not remove:
            print(f"#{n}: in sync {sorted(want)}", file=out)
            continue
        verb = "" if apply else "would "
        print(f"#{n}: header {sorted(want)} links {sorted(have)}; {verb}add {add} {verb}remove {remove}", file=out)
        if not apply:
            continue
        for b in remove:
            gh("api", "-X", "DELETE", f"{api}/issues/{n}/dependencies/blocked_by/{have[b]}")
        for b in add:
            try:
                blocker_id = json.loads(gh("api", f"{api}/issues/{b}"))["id"]
                gh("api", "-X", "POST", f"{api}/issues/{n}/dependencies/blocked_by", "-F", f"issue_id={blocker_id}")
            except GhError as err:
                if not already_taken(err):
                    raise
                print(f"#{n}: #{b} already in sync: {err}", file=out)


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--milestone", required=True, help="GitHub milestone title")
    parser.add_argument("--tracking", required=True, type=int, help="tracking issue number, skipped")
    parser.add_argument("--repo", help="owner/name (default: the current gh repo)")
    parser.add_argument("--apply", action="store_true", help="change the links (default: dry run)")
    args = parser.parse_args(argv)
    try:
        repo = args.repo or gh("repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner").strip()
        sync(repo, args.milestone, args.tracking, args.apply, sys.stdout)
    except GhError as err:
        print(f"error: {err}", file=sys.stderr)
        return 1
    if not args.apply:
        print("dry run: rerun with --apply to change the links")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
