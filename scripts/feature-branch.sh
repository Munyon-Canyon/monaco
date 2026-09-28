#!/usr/bin/env bash
set -euo pipefail

REPO="${MONACO_REPO:-lognorman20/monaco}"
ACTIONS_APP_ID=15368
VERIFIER_APP_ID=5101392

usage() {
  cat >&2 <<'USAGE'
usage: scripts/feature-branch.sh init|apply|ruleset <name>
  init <name>     create <name> from origin/main, then apply
  apply <name>    add the Graphite trunk, turn on auto-merge, create or update the ruleset
  ruleset <name>  print the ruleset body
See docs/architecture/ci.md#feature-branches.
USAGE
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
  gt trunk --add "$name" --no-interactive
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
