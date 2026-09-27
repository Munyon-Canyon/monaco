#!/usr/bin/env python3
"""Fail a pull request whose title or body does not follow the repo's PR format.

Title: what the PR changes, present tense. No leading issue number and no
commit-type prefix such as "docs:" or "feat(x):".
Body: the six sections of .github/pull_request_template.md, each with real text
after HTML comments are removed.

Reads PR_TITLE and PR_BODY from the environment. Rules:
docs/architecture/backend-platform.md#pull-requests-small-and-stacked
"""

import os
import re
import sys

SECTIONS = ["TLDR", "Why", "What changed", "Proof", "What came up", "Reviewer focus"]
ISSUE_PREFIX_RE = re.compile(r"^\s*#\d+")
COMMIT_PREFIX_RE = re.compile(r"^\s*[a-z]+(\([^)]*\))?!?:\s")


def title_errors(title: str) -> list[str]:
    if not title.strip():
        return ["title is empty"]
    if ISSUE_PREFIX_RE.match(title):
        return [f'title starts with an issue number; link the issue under Why instead: "{title}"']
    if COMMIT_PREFIX_RE.match(title):
        return [f'title starts with a commit-type prefix; say what the PR changes instead: "{title}"']
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


def main() -> int:
    errors = title_errors(os.environ.get("PR_TITLE", "")) + body_errors(os.environ.get("PR_BODY", ""))
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
