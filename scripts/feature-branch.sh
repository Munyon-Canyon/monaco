#!/usr/bin/env bash
# Milestone feature branches: a Graphite trunk that only takes squash-merged PRs with green
# `ci / ci-ok` and `verify`. docs/architecture/ci.md#feature-branches
#
#   scripts/feature-branch.sh init <name>      create <name> from origin/main, then apply
#   scripts/feature-branch.sh apply <name>     add the Graphite trunk and create or update the ruleset
#   scripts/feature-branch.sh ruleset <name>   print the ruleset body
set -euo pipefail

REPO="${MONACO_REPO:-lognorman20/monaco}"
ACTIONS_APP_ID=15368
VERIFIER_APP_ID=5101392

usage() {
  echo "usage: scripts/feature-branch.sh init|apply|ruleset <name>" >&2
  exit 2
}

ruleset() {
  jq -n --arg name "$1" --argjson actions "$ACTIONS_APP_ID" --argjson verifier "$VERIFIER_APP_ID" '{
    name: "feature branch \($name)",
    target: "branch",
    enforcement: "active",
    bypass_actors: [],
    conditions: {ref_name: {include: ["refs/heads/\($name)"], exclude: []}},
    rules: [
      {type: "deletion"},
      {type: "non_fast_forward"},
      {type: "pull_request", parameters: {
        required_approving_review_count: 0,
        dismiss_stale_reviews_on_push: false,
        require_code_owner_review: false,
        require_last_push_approval: false,
        require_extra_approval_for_unattributed_changes: false,
        required_review_thread_resolution: false,
        allowed_merge_methods: ["squash"]
      }},
      {type: "required_status_checks", parameters: {
        strict_required_status_checks_policy: true,
        do_not_enforce_on_create: false,
        required_status_checks: [
          {context: "ci / ci-ok", integration_id: $actions},
          {context: "verify", integration_id: $verifier}
        ]
      }}
    ]
  }'
}

apply() {
  local name="$1" id
  gt trunk --add "$name" --no-interactive 2>/dev/null || gt trunk --all | grep -qx "$name"
  gh api -X PATCH "repos/$REPO" -F allow_auto_merge=true --silent
  id="$(gh api "repos/$REPO/rulesets" --jq ".[] | select(.name == \"feature branch $name\") | .id")"
  if [[ -n "$id" ]]; then
    ruleset "$name" | gh api -X PUT "repos/$REPO/rulesets/$id" --input - --jq '.id'
  else
    ruleset "$name" | gh api -X POST "repos/$REPO/rulesets" --input - --jq '.id'
  fi
}

init() {
  local name="$1"
  git fetch --quiet origin main
  if ! git ls-remote --exit-code --heads origin "$name" >/dev/null; then
    gh api "repos/$REPO/git/refs" -f ref="refs/heads/$name" -f sha="$(git rev-parse origin/main)" --silent
  fi
  git fetch --quiet origin "$name"
  git branch --force "$name" "origin/$name"
  apply "$name"
}

[[ $# -eq 2 && -n "$2" ]] || usage
case "$1" in
  init) init "$2" ;;
  apply) apply "$2" ;;
  ruleset) ruleset "$2" ;;
  *) usage ;;
esac
