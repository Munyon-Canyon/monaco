#!/usr/bin/env bash
set -euo pipefail

REPO="${MONACO_REPO:-Munyon-Canyon/monaco}"
ACTIONS_APP_ID=15368

usage() {
  cat >&2 <<'USAGE'
usage: scripts/feature-branch.sh init|apply|ruleset <name> | main-ruleset
  init <name>     create <name> from origin/main, then apply
  apply <name>    add the Graphite trunk, turn on auto-merge and merge commits,
                  create or update the feature branch and main rulesets
  ruleset <name>  print the feature branch ruleset body
  main-ruleset    print the main ruleset body
See docs/architecture/ci.md#feature-branches.
USAGE
  exit 2
}

ruleset() {
  jq -n --arg name "$1" --argjson actions "$ACTIONS_APP_ID" '{
    name: "feature branch \($name)",
    target: "branch",
    enforcement: "active",
    bypass_actors: [{actor_id: 1, actor_type: "OrganizationAdmin", bypass_mode: "always"}],
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
        allowed_merge_methods: ["merge"]
      }},
      {type: "merge_queue", parameters: {
        merge_method: "MERGE",
        grouping_strategy: "ALLGREEN",
        max_entries_to_build: 5,
        min_entries_to_merge: 1,
        max_entries_to_merge: 5,
        min_entries_to_merge_wait_minutes: 0,
        check_response_timeout_minutes: 30
      }},
      {type: "required_status_checks", parameters: {
        strict_required_status_checks_policy: false,
        do_not_enforce_on_create: false,
        required_status_checks: [
          {context: "ci / ci-ok", integration_id: $actions},
          {context: "PR format (title, body and commits)", integration_id: $actions}
        ]
      }}
    ]
  }'
}

main_ruleset() {
  jq -n --argjson actions "$ACTIONS_APP_ID" '{
    name: "main",
    target: "branch",
    enforcement: "active",
    bypass_actors: [],
    conditions: {ref_name: {include: ["~DEFAULT_BRANCH"], exclude: []}},
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
          {context: "Changelog (checkpoint into main)", integration_id: $actions}
        ]
      }}
    ]
  }'
}

put_ruleset() {
  local name="$1" id
  id="$(gh api "repos/$REPO/rulesets" --jq ".[] | select(.name == \"$name\") | .id")"
  if [[ -n "$id" ]]; then
    gh api -X PUT "repos/$REPO/rulesets/$id" --input - --jq '.id'
  else
    gh api -X POST "repos/$REPO/rulesets" --input - --jq '.id'
  fi
}

apply() {
  local name="$1"
  gt trunk --add "$name" --no-interactive
  gh api -X PATCH "repos/$REPO" -F allow_auto_merge=true -F allow_merge_commit=true --silent
  gh variable set FEATURE_BRANCH --repo "$REPO" --body "$name"
  ruleset "$name" | put_ruleset "feature branch $name"
  main_ruleset | put_ruleset main
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

[[ $# -eq 1 && "$1" == main-ruleset ]] && { main_ruleset; exit; }
[[ $# -eq 2 && -n "$2" ]] || usage
case "$1" in
  init) init "$2" ;;
  apply) apply "$2" ;;
  ruleset) ruleset "$2" ;;
  *) usage ;;
esac
