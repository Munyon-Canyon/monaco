#!/usr/bin/env bash
# Stage 1 reuse, docs/architecture/ci.md#check-stages. Writes patch-id, ci-id and reuse to $GITHUB_OUTPUT.
set -euo pipefail

: "${BASE_SHA:?}" "${HEAD_SHA:?}" "${HEAD_REF:?}" "${GITHUB_REPOSITORY:?}" "${GITHUB_OUTPUT:?}"

patch_id=""
read -r patch_id _ < <(git diff "$BASE_SHA...$HEAD_SHA" | git patch-id --stable) || true
echo "patch-id=$patch_id" >>"$GITHUB_OUTPUT"

ci_tree="$(git rev-parse "$HEAD_SHA:.github/workflows")"
ci_id="${ci_tree:0:12}"
echo "ci-id=$ci_id" >>"$GITHUB_OUTPUT"
echo "patch ID of $BASE_SHA...$HEAD_SHA: ${patch_id:-none}"
echo "ci-id of $HEAD_SHA:.github/workflows: $ci_id"
if [[ -z "$patch_id" ]]; then
  echo "reuse=false" >>"$GITHUB_OUTPUT"
  exit 0
fi

last=""
shas="$(gh api "repos/$GITHUB_REPOSITORY/actions/runs?branch=$HEAD_REF&event=pull_request&per_page=50" \
  --jq '.workflow_runs[].head_sha' | awk '!seen[$0]++')"
for sha in $shas; do
  last="$(gh api "repos/$GITHUB_REPOSITORY/commits/$sha/check-runs?check_name=ci%20%2F%20ci-ok&status=completed" \
    --jq 'first(.check_runs[] | select(.conclusion == "success")) | .output.summary // "no patch ID"')"
  if [[ -n "$last" ]]; then
    echo "last green ci / ci-ok: $sha, $last"
    break
  fi
done

if [[ "$last" == "patch-id: $patch_id ci-id: $ci_id" ]]; then
  echo "reuse=true" >>"$GITHUB_OUTPUT"
  echo "unchanged diff and workflow tree: every stage 1 job skips"
else
  echo "reuse=false" >>"$GITHUB_OUTPUT"
fi
