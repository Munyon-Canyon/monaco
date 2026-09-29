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
view="$(gh pr view "$pr" --json isDraft,baseRefName,headRefName,baseRefOid,headRefOid \
  --jq '[.isDraft, .baseRefName, .headRefName, .baseRefOid, .headRefOid] | @tsv')"
read -r draft base_ref head_ref base_sha head_sha <<<"$view"
git cat-file -e "$base_sha^{commit}" 2>/dev/null || git fetch --quiet origin "$base_ref"
git cat-file -e "$head_sha^{commit}" 2>/dev/null || git fetch --quiet origin "$head_ref"

PR_TITLE="$title" PR_BODY="$body" BASE_REF="$base_ref" HEAD_REF="$head_ref" BASE_SHA="$base_sha" HEAD_SHA="$head_sha" \
  python3 "$here/check-pr-format.py" >&2
gh pr edit "$pr" --title "$title" --body-file "$file"
if [[ "$draft" == "true" ]]; then
  gh pr ready "$pr"
fi
