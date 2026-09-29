#!/usr/bin/env python3

from __future__ import annotations

import json
import os
import re
import subprocess
import sys
from typing import NamedTuple

SECTIONS = ["TLDR", "Why", "What changed", "Proof", "What came up", "Reviewer focus"]
ISSUE_PREFIX_RE = re.compile(r"^\s*#\d+")
COMMIT_PREFIX_RE = re.compile(r"^\s*[a-z]+(\([^)]*\))?!?:\s")
NEEDS = "Needs from Logan"
LINK_RE = re.compile(r"(?i)\b(part of|close[sd]?|fix(?:e[sd])?|resolve[sd]?)\s+#(\d+)\b")
SHA_RE = re.compile(r"(?<![\w-])(?=[0-9a-f]*[0-9])(?=[0-9a-f]*[a-f])[0-9a-f]{7,40}(?![\w-])")
FENCE_RE = re.compile(r"(?ms)^(`{3,}|~{3,})[ \t]*([\w+-]*)[^\n]*\n(.*?)^\1[ \t]*$")
SHELL_FENCES = {"", "sh", "bash", "shell", "console", "zsh"}
CONVENTIONAL_RE = re.compile(r"^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)(\([^()\s]+\))?!?: \S")
SQUASH_RE = re.compile(r"\(#\d+\)$")
FEATURE_BRANCH_RE = re.compile(r"^[a-z0-9]+(-[a-z0-9]+)*-checkpoint-[0-9]+$")


class StackedPR(NamedTuple):
    number: int
    body: str


def title_errors(title: str) -> list[str]:
    if not title.strip():
        return ["title is empty"]
    if ISSUE_PREFIX_RE.match(title):
        return [f'title starts with an issue number; link the issue under Why instead: "{title}"']
    if COMMIT_PREFIX_RE.match(title):
        return [f'title starts with a commit-type prefix; say what the PR changes instead: "{title}"']
    return []


def strip_comments(body: str) -> str:
    return re.sub(r"<!--.*?-->", "", body or "", flags=re.S)


def section(body: str, name: str) -> str | None:
    text = strip_comments(body)
    m = re.search(rf"(?m)^## {re.escape(name)}[ \t]*$", text)
    if not m:
        return None
    nxt = re.search(r"(?m)^## ", text[m.end():])
    return text[m.end(): m.end() + nxt.start() if nxt else len(text)]


def closes_by_ticket(body: str) -> dict[int, bool]:
    found: dict[int, bool] = {}
    for verb, number in LINK_RE.findall(section(body, "Why") or ""):
        found[int(number)] = found.get(int(number), False) or verb.lower() != "part of"
    return found


def ticket_errors(body: str, stacked_on: list[StackedPR], stacked_under: list[StackedPR]) -> list[str]:
    mine = closes_by_ticket(body)
    if not mine:
        return ['"## Why" links no ticket; write "Part of #n", or "Closes #n" on the ticket\'s last PR']
    errors = []
    for ticket, closes in sorted(mine.items()):
        if closes:
            for number, other in stacked_on:
                if ticket in closes_by_ticket(other):
                    errors.append(
                        f"this PR closes #{ticket} but #{number} above it is part of #{ticket}; "
                        f'only the ticket\'s last PR says "Closes #{ticket}", this one says "Part of #{ticket}"'
                    )
        for number, other in stacked_under:
            if closes_by_ticket(other).get(ticket):
                errors.append(
                    f"#{number} below this PR closes #{ticket}; only the ticket's last PR closes it, "
                    f'so #{number} should say "Part of #{ticket}"'
                )
    if any(mine.values()):
        needs = section(body, NEEDS)
        if needs is None:
            errors.append(f'this PR closes its ticket, so it needs a "## {NEEDS}" section ("Nothing." or a checklist)')
        elif not needs.strip():
            errors.append(f'"## {NEEDS}" is empty; write "Nothing." or the checklist')
    return errors


def command_errors(body: str) -> list[str]:
    errors = []
    for _, lang, code in FENCE_RE.findall(section(body, NEEDS) or ""):
        if lang.lower() not in SHELL_FENCES:
            continue
        run = subprocess.run(["bash", "-n"], input=code, capture_output=True, text=True)
        if run.returncode != 0:
            errors.append(f'a command under "## {NEEDS}" does not parse: {run.stderr.strip()}')
    return errors


def sha_errors(body: str, head: str) -> list[str]:
    errors = []
    for sha in dict.fromkeys(SHA_RE.findall(strip_comments(body))):
        full = git("rev-parse", "--verify", "--quiet", f"{sha}^{{commit}}")
        if full is None:
            errors.append(f"the body cites {sha}, which is not a commit in this repository")
        elif git("merge-base", "--is-ancestor", full, head) is None:
            errors.append(f"the body cites {sha}, which is not an ancestor of the head {head[:12]}; cite the SHA this PR contains")
    return errors


def commit_errors(base: str, head: str) -> list[str]:
    log = git("log", "--format=%H%x09%P%x09%s", f"{base}..{head}")
    if log is None:
        return [f"cannot list the commits in {base[:12]}..{head[:12]}"]
    errors = []
    for line in log.splitlines():
        sha, parents, subject = line.split("\t", 2)
        if len(parents.split()) > 1 or SQUASH_RE.search(subject) or CONVENTIONAL_RE.match(subject):
            continue
        errors.append(
            f'commit {sha[:12]} "{subject}" is not a Conventional Commit (type(scope)!: subject). '
            "Reword it with the /commit skill"
        )
    return errors


def git(*args: str) -> str | None:
    run = subprocess.run(["git", *args], capture_output=True, text=True)
    return run.stdout.strip() if run.returncode == 0 else None


def stacked(flag: str, ref: str) -> list[StackedPR]:
    out = subprocess.run(
        ["gh", "pr", "list", "--state", "open", flag, ref, "--json", "number,body"],
        capture_output=True, text=True, check=True,
    ).stdout
    return [StackedPR(pr["number"], pr["body"]) for pr in json.loads(out)]


def body_errors(body: str) -> list[str]:
    text = strip_comments(body)
    errors = []
    headings = [(m.group(1).strip(), m.start(), m.end()) for m in re.finditer(r"(?m)^## (.+)$", text)]
    names = [h[0] for h in headings]
    for section in SECTIONS:
        if section not in names:
            errors.append(f'body is missing the "## {section}" section')
    for i, (name, _, end) in enumerate(headings):
        if name not in SECTIONS:
            continue
        stop = headings[i + 1][1] if i + 1 < len(headings) else len(text)
        if not text[end:stop].strip():
            errors.append(f'"## {name}" is empty; write the content or "None."')
    return errors


def is_checkpoint(base_ref: str, head_ref: str, labels: list[str]) -> bool:
    return base_ref == "main" and bool(FEATURE_BRANCH_RE.fullmatch(head_ref)) and "integration" in labels


def main(argv: list[str]) -> int:
    env = os.environ
    body = env.get("PR_BODY", "")
    if argv:
        print("usage: check-pr-format.py", file=sys.stderr)
        return 2
    errors = title_errors(env.get("PR_TITLE", "")) + body_errors(body) + command_errors(body)
    errors += ticket_errors(body, stacked("--base", env["HEAD_REF"]), stacked("--head", env["BASE_REF"]))
    errors += sha_errors(body, env["HEAD_SHA"])
    # A checkpoint's range is the whole milestone: each commit passed this check in its ticket PR or predates the rule.
    if not is_checkpoint(env["BASE_REF"], env["HEAD_REF"], json.loads(env.get("PR_LABELS") or "[]")):
        errors += commit_errors(env["BASE_SHA"], env["HEAD_SHA"])
    if errors:
        print("PR format check failed:")
        for e in errors:
            print(f"  - {e}")
        print("Format: .github/pull_request_template.md. Rules: docs/architecture/backend-platform.md#pull-requests-small-and-stacked")
        return 1
    print("PR format ok")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
