#!/usr/bin/env bash
set -euo pipefail

REPO="${MONACO_REPO:-Munyon-Canyon/monaco}"
ACTIONS_APP_ID=15368

usage() {
  cat >&2 <<'USAGE'
usage: scripts/feature-branch.sh init|apply|ruleset <feature>-checkpoint-<N> | main-ruleset
  init <branch>     create <branch> from origin/main, then apply; start a feature
                    with init <feature>-checkpoint-1
  apply <branch>    add the Graphite trunk, turn on auto-merge and merge commits,
                    add refs/heads/<branch> to the feature branch ruleset, create
                    or update both rulesets
  ruleset <branch>  print a feature branch ruleset body that targets refs/heads/<branch>
  main-ruleset      print the main ruleset body
See docs/architecture/ci.md#feature-branches.
USAGE
  exit 2
}

# A merge_queue rule takes exact ref names only, so the ruleset lists each feature branch.
ref() {
  "$(dirname "$0")/ci/feature-branch-name.sh" "$1" >/dev/null || {
    echo "$1 is not a feature branch <feature>-checkpoint-<N> (lowercase slug, hyphen, number)" >&2
    exit 2
  }
  echo "refs/heads/$1"
}

ruleset() {
  jq -n --argjson include "$1" --argjson actions "$ACTIONS_APP_ID" '{
    name: "feature branches",
    target: "branch",
    enforcement: "active",
    bypass_actors: [{actor_id: 1, actor_type: "OrganizationAdmin", bypass_mode: "always"}],
    conditions: {ref_name: {include: $include, exclude: []}},
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

ruleset_id() {
  gh api "repos/$REPO/rulesets" --jq "[.[] | select(.name | test(\"$1\"))][0].id // empty"
}

put_ruleset() {
  local id="$1"
  if [[ -n "$id" ]]; then
    gh api -X PUT "repos/$REPO/rulesets/$id" --input - --jq '.id'
  else
    gh api -X POST "repos/$REPO/rulesets" --input - --jq '.id'
  fi
}

# One ruleset covers every feature branch: apply adds refs/heads/<branch> to its include list and keeps the
# refs there. "^feature branch" also matches an older per-branch ruleset ("feature branch <branch>"), which
# apply renames in place.
apply() {
  local name="$1" include="[\"$2\"]" id
  gt trunk --add "$name" --no-interactive
  gh api -X PATCH "repos/$REPO" -F allow_auto_merge=true -F allow_merge_commit=true --silent
  id="$(ruleset_id "^feature branch")"
  if [[ -n "$id" ]]; then
    include="$(gh api "repos/$REPO/rulesets/$id" \
      --jq ".conditions.ref_name.include as \$i | if any(\$i[]; . == \"$2\") then \$i else \$i + [\"$2\"] end")"
  fi
  ruleset "$include" | put_ruleset "$id"
  main_ruleset | put_ruleset "$(ruleset_id "^main$")"
}

init() {
  local name="$1"
  git fetch --quiet origin main
  if ! git ls-remote --exit-code --heads origin "$name" >/dev/null; then
    gh api "repos/$REPO/git/refs" -f ref="refs/heads/$name" -f sha="$(git rev-parse origin/main)" --silent
  fi
  git fetch --quiet origin "$name"
  git branch --force "$name" "origin/$name"
  apply "$name" "$2"
}

case "$#:${1:-}" in
  1:main-ruleset) main_ruleset; exit ;;
  2:ruleset | 2:init | 2:apply) ;;
  *) usage ;;
esac
ref="$(ref "$2")"
case "$1" in
  ruleset) ruleset "[\"$ref\"]" ;;
  init) init "$2" "$ref" ;;
  apply) apply "$2" "$ref" ;;
esac
