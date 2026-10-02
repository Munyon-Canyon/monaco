#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 3 ]]; then
  echo "usage: scripts/pr-body.sh <pr> <title> <body-file>" >&2
  exit 64
fi
pr="$1"
title="$2"
file="$3"
if [[ ! -s "$file" ]]; then
  echo "pr-body: $file is missing or empty" >&2
  exit 1
fi
body="$(cat "$file")"

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -n "${GH_REPO:-}" ]]; then
  repo="$GH_REPO"
else
  repo="$(
    gh repo view --json nameWithOwner --jq .nameWithOwner
  )"
fi
view="$(gh api "repos/${repo}/pulls/${pr}" --jq '[.draft, .base.ref, .head.ref, .base.sha, .head.sha] | @tsv')"
read -r draft base_ref head_ref base_sha head_sha <<<"$view"
git cat-file -e "$base_sha^{commit}" 2>/dev/null || git fetch --quiet origin "$base_ref"
git cat-file -e "$head_sha^{commit}" 2>/dev/null || git fetch --quiet origin "$head_ref"

PR_TITLE="$title" PR_BODY="$body" BASE_REF="$base_ref" HEAD_REF="$head_ref" BASE_SHA="$base_sha" HEAD_SHA="$head_sha" \
  python3 "$here/check-pr-format.py" >&2
# Ready first: an edit on a draft starts a PR format run that skips, and it cancels the real run.
if [[ "$draft" == "true" ]]; then
  ready_out="$(gh pr ready "$pr" 2>&1)" || {
    status=$?
    if [[ "$ready_out" == *403* || "$ready_out" == *"not permitted"* ]]; then
      gh api -X POST "repos/${repo}/pulls/${pr}/ccr/ready_for_review"
    else
      printf '%s\n' "$ready_out" >&2
      exit "$status"
    fi
  }
fi
gh api -X PATCH "repos/${repo}/pulls/${pr}" -f title="$title" -F "body=@${file}"
