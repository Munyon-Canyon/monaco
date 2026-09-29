#!/usr/bin/env bash
set -euo pipefail

[[ $# -eq 2 ]] || { echo "usage: scripts/ci/next-branch.sh <checkpoint-head-ref> <squash-sha>" >&2; exit 2; }
REPO="${GITHUB_REPOSITORY:-Munyon-Canyon/monaco}"
head_ref="$1" squash="$2"

if [[ -z "${GH_TOKEN:-}" ]]; then
  echo "::error::Add the MERGE_BACK_TOKEN repo secret: a fine-grained PAT from an org admin with contents: write. Only org admins bypass the feature branch ruleset."
  exit 1
fi
if ! parsed="$("$(dirname "$0")/feature-branch-name.sh" "$head_ref")"; then
  echo "::error::The checkpoint head $head_ref is not a feature branch <feature>-checkpoint-<N>, so there is no next feature branch to cut."
  exit 1
fi
read -r feature n <<<"$parsed"
next="$feature-checkpoint-$((10#$n + 1))"

if at="$(gh api "repos/$REPO/git/ref/heads/$next" --jq .object.sha 2>/dev/null)"; then
  if [[ "$at" != "$squash" ]]; then
    echo "::error::$next already exists at $at, not at the checkpoint squash $squash. Delete or rename it, then rerun this job."
    exit 1
  fi
  echo "$next already exists at $squash"
else
  gh api "repos/$REPO/git/refs" -f ref="refs/heads/$next" -f sha="$squash" --silent
  echo "Created $next at $squash"
fi

if ! gh variable set FEATURE_BRANCH --repo "$REPO" --body "$next"; then
  echo "::warning::Could not set the FEATURE_BRANCH repo variable to $next. Give MERGE_BACK_TOKEN the Variables: write repository permission, or run: gh variable set FEATURE_BRANCH --body $next"
fi

# A merge_queue rule takes exact ref names only, so the new branch joins the feature branch ruleset by name.
add_to_ruleset() {
  local id body
  id="$(gh api "repos/$REPO/rulesets" --jq '[.[] | select(.name | test("^feature branch"))][0].id // empty')" || return 1
  [[ -n "$id" ]] || return 1
  body="$(gh api "repos/$REPO/rulesets/$id")" || return 1
  if jq -e --arg ref "$ref" '.conditions.ref_name.include | any(.[]; . == $ref)' <<<"$body" >/dev/null; then
    printf -v ruleset_result "The feature branch ruleset already includes \`%s\`." "$ref"
    return
  fi
  jq --arg ref "$ref" '{name, target, enforcement, bypass_actors, conditions, rules} | .conditions.ref_name.include += [$ref]' <<<"$body" |
    gh api -X PUT "repos/$REPO/rulesets/$id" --input - --silent || return 1
  printf -v ruleset_result "Added \`%s\` to the feature branch ruleset." "$ref"
}

ref="refs/heads/$next"
if add_to_ruleset; then
  echo "$ruleset_result"
else
  printf -v ruleset_result "Could not add \`%s\` to the feature branch ruleset. Run \`scripts/feature-branch.sh apply %s\`." "$ref" "$next"
  echo "::warning::Could not add $ref to the feature branch ruleset, so $next has no merge queue, required checks or deletion protection. Give MERGE_BACK_TOKEN the Administration: write repository permission, or run: scripts/feature-branch.sh apply $next"
fi

stranded=()
if open="$(gh pr list --repo "$REPO" --base main --state open --json number,labels \
  --jq '.[] | select(any(.labels[]; .name == "integration") | not) | .number')"; then
  while read -r number; do
    [[ -z "$number" ]] && continue
    stranded+=("$number")
    echo "::warning::PR #$number is now based on main after the checkpoint; move it onto $next with gt track --parent $next && gt restack --upstack && gt submit"
  done <<<"$open"
else
  echo "::warning::Could not list open PRs based on main. Check for ticket PRs GitHub retargeted from the deleted $head_ref and move them onto $next."
fi

if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
  {
    echo "### Next feature branch: \`$next\`"
    echo
    echo "Cut from the checkpoint squash $squash on main. Run \`gt trunk --add $next\` and continue there."
    echo
    echo "$ruleset_result"
    if ((${#stranded[@]})); then
      echo
      echo "These open PRs are now based on main. Move each onto \`$next\` with \`gt track --parent $next && gt restack --upstack && gt submit\`:"
      echo
      for number in "${stranded[@]}"; do echo "- #$number"; done
    fi
  } >>"$GITHUB_STEP_SUMMARY"
fi
