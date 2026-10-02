#!/usr/bin/env bash
set -euo pipefail

REPO="${MONACO_REPO:-Munyon-Canyon/monaco}"
ACTIONS_APP_ID=15368
GRAPHITE_APP_ID=158384

usage() {
  cat >&2 <<'USAGE'
usage: scripts/branch-rulesets.sh staging-ruleset | main-ruleset | apply
  staging-ruleset  print the staging ruleset body
  main-ruleset     print the main ruleset body
  apply            create or update both rulesets
See docs/architecture/ci.md#feature-branches.
USAGE
  exit 2
}

# PRs land in staging through the Graphite merge queue, squashed. Graphite's queue optimizations push to
# staging, so the Graphite App bypasses the ruleset.
staging_ruleset() {
  jq -n --argjson actions "$ACTIONS_APP_ID" --argjson graphite "$GRAPHITE_APP_ID" '{
    name: "staging",
    target: "branch",
    enforcement: "active",
    bypass_actors: [
      {actor_id: 1, actor_type: "OrganizationAdmin", bypass_mode: "always"},
      {actor_id: $graphite, actor_type: "Integration", bypass_mode: "always"}
    ],
    conditions: {ref_name: {include: ["refs/heads/staging"], exclude: []}},
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

# A promotion merges staging into main with a merge commit, so the two histories stay linked.
main_ruleset() {
  jq -n --argjson actions "$ACTIONS_APP_ID" '{
    name: "main",
    target: "branch",
    enforcement: "active",
    bypass_actors: [],
    conditions: {ref_name: {include: ["refs/heads/main"], exclude: []}},
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

ruleset_id() {
  gh api "repos/$REPO/rulesets" --jq "[.[] | select(.name == \"$1\")][0].id // empty"
}

put_ruleset() {
  local id="$1"
  if [[ -n "$id" ]]; then
    gh api -X PUT "repos/$REPO/rulesets/$id" --input - --jq '.id'
  else
    gh api -X POST "repos/$REPO/rulesets" --input - --jq '.id'
  fi
}

apply() {
  staging_ruleset | put_ruleset "$(ruleset_id staging)"
  main_ruleset | put_ruleset "$(ruleset_id main)"
}

[[ $# -eq 1 ]] || usage
case "$1" in
  staging-ruleset) staging_ruleset ;;
  main-ruleset) main_ruleset ;;
  apply) apply ;;
  *) usage ;;
esac
