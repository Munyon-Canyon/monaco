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
  echo "::error::The checkpoint head $head_ref is not a feature branch <name>-<N>, so there is no next feature branch to cut."
  exit 1
fi
read -r name n <<<"$parsed"
next="$name-$((10#$n + 1))"

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
    if ((${#stranded[@]})); then
      echo
      echo "These open PRs are now based on main. Move each onto \`$next\` with \`gt track --parent $next && gt restack --upstack && gt submit\`:"
      echo
      for number in "${stranded[@]}"; do echo "- #$number"; done
    fi
  } >>"$GITHUB_STEP_SUMMARY"
fi
