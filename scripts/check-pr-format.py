#!/usr/bin/env python3
"""Fail a pull request whose title or body does not follow the repo's PR format.

Title: "#<issue> <what changes>", where <issue> is an open or closed issue (not a PR).
Body: the six sections of .github/pull_request_template.md, each with real text
after HTML comments are removed.

Reads PR_TITLE, PR_BODY and GITHUB_REPOSITORY from the environment. Uses the gh CLI
to confirm the issue exists. Rules: docs/architecture/backend-platform.md
#pull-requests-small-and-stacked
"""

import json
import os
import re
import subprocess
import sys

SECTIONS = ["TLDR", "Why", "What changed", "Proof", "What came up", "Reviewer focus"]
TITLE_RE = re.compile(r"^#(\d+) \S")


def title_errors(title: str, repo: str, check_issue) -> list[str]:
    m = TITLE_RE.match(title)
    if not m:
        return [f'title must look like "#<issue> <what changes>", got: "{title}"']
    number = m.group(1)
    kind = check_issue(repo, number)
    if kind == "missing":
        return [f"title names #{number}, which does not exist in {repo}"]
    if kind == "pull_request":
        return [f"title names #{number}, which is a pull request, not an issue"]
    return []


def body_errors(body: str) -> list[str]:
    text = re.sub(r"<!--.*?-->", "", body or "", flags=re.S)
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


def gh_issue_kind(repo: str, number: str) -> str:
    result = subprocess.run(
        ["gh", "api", f"repos/{repo}/issues/{number}"],
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        return "missing"
    return "pull_request" if "pull_request" in json.loads(result.stdout) else "issue"


def main() -> int:
    title = os.environ.get("PR_TITLE", "")
    body = os.environ.get("PR_BODY", "")
    repo = os.environ.get("GITHUB_REPOSITORY", "")
    errors = title_errors(title, repo, gh_issue_kind) + body_errors(body)
    if errors:
        print("PR format check failed:")
        for e in errors:
            print(f"  - {e}")
        print("Format: .github/pull_request_template.md. Rules: docs/architecture/backend-platform.md#pull-requests-small-and-stacked")
        return 1
    print("PR format ok")
    return 0


if __name__ == "__main__":
    sys.exit(main())
