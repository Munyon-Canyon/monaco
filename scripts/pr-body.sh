#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "usage: scripts/pr-body.sh <pr> <file>" >&2
  exit 64
fi
pr="$1"
file="$2"
if [[ ! -s "$file" ]]; then
  echo "pr-body: $file is missing or empty" >&2
  exit 1
fi

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
title="$(gh pr view "$pr" --json title --jq .title)"
PR_TITLE="$title" PR_BODY="$(cat "$file")" python3 "$here/check-pr-format.py" >&2
gh pr edit "$pr" --body-file "$file"
