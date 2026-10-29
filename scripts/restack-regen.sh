#!/usr/bin/env bash
# Settles a stopped `gt restack` (or rebase) whose every conflicted path is generated: takes the
# upstream copy of each, runs the generators, stages the result and continues, until the rebase ends.
# With no rebase in progress it runs `gt restack` first. Any hand-written conflict stops it.
# When the restack ends, a migration the stack adds that now sorts at or below one the trunk gained is
# renamed with gen migration --rebase (which also rewrites atlas.sum) and the branch amended, so a
# restack never leaves the ready row's migrate order failing (lanes 6462 and 6507 on 2026-10-29).
# GT, GENERATE, GENERATED_GLOBS, MIGRATE_ORDER and MIGRATE_REBASE let the test swap in fakes.
set -euo pipefail

gt=${GT:-gt}
backend_go="cd apps/backend && GOTOOLCHAIN=\$(sed -n 's/^toolchain //p' go.mod) && test -n \"\$GOTOOLCHAIN\" && export GOTOOLCHAIN &&"
generate=${GENERATE:-"$backend_go go generate ./..."}
migrate_order=${MIGRATE_ORDER:-"$backend_go go run ./cmd/monacoctl migrate order"}
migrate_rebase=${MIGRATE_REBASE:-"$backend_go go run ./cmd/monacoctl gen migration --rebase"}
# The paths .gitattributes marks linguist-generated, plus atlas.sum.
generated=${GENERATED_GLOBS:-"apps/backend/api/openapi.yaml
apps/backend/internal/platform/httpx/api/*.gen.go
apps/backend/internal/modules/*/sqlc/*.gen.go
apps/backend/internal/platform/db/sqlc/*.go
apps/backend/cmd/*.gen.go
apps/backend/internal/platform/observability/msgs_*.gen.go
apps/backend/.golangci.yml
apps/backend/migrations/atlas.sum
packages/flows/Sources/MonacoFlows/*.gen.swift
packages/mobile-core/Tests/MonacoCoreTests/Flows/*.gen.swift
packages/mobile-core/Tests/MonacoAPITests/ErrorCodeCases.gen.swift
.claude/skills/verify-backend/feature-map/*.md
docs/reference/events.md
docs/reference/errors.md
docs/reference/logs.md"}

cd "$(git rev-parse --show-toplevel)"

rebasing() {
  [[ -d "$(git rev-parse --git-path rebase-merge)" || -d "$(git rev-parse --git-path rebase-apply)" ]]
}

settle_migrations() {
  local report
  report=$(bash -c "$migrate_order" 2>&1) && return 0
  echo "restack-regen: the restack left a migration out of order:"
  printf '  %s\n' "$report"
  bash -c "$migrate_rebase"
  git add -A apps/backend/migrations
  echo "restack-regen: renamed:"
  git diff --cached --name-status -M -- apps/backend/migrations | sed 's/^/  /'
  "$gt" modify --no-interactive
  if ! report=$(bash -c "$migrate_order" 2>&1); then
    echo "restack-regen: still out of order after the rename (a lower branch adds it?); rebase it there:" >&2
    printf '  %s\n' "$report" >&2
    return 1
  fi
}

conflicted_paths() {
  git diff --name-only --diff-filter=U
}

is_generated() {
  local glob
  while IFS= read -r glob; do
    # Unquoted on the right so the case pattern's * matches across slashes, as the ** globs of .gitattributes do.
    # shellcheck disable=SC2254
    case "$1" in $glob) return 0 ;; esac
  done <<<"$generated"
  return 1
}

if ! rebasing; then
  "$gt" restack || true
fi

stalled=0
for _ in $(seq 1 100); do
  if ! rebasing; then
    echo "restack-regen: the restack finished"
    settle_migrations
    exit
  fi
  conflicted=$(conflicted_paths)
  if [[ -z "$conflicted" ]]; then
    # Nothing to settle, so only a failed continue can keep the rebase here.
    stalled=$((stalled + 1))
    if ((stalled > 2)); then
      echo "restack-regen: the rebase stopped with no conflicted path; resolve it by hand" >&2
      exit 1
    fi
  else
    stalled=0
    hand=()
    while IFS= read -r path; do
      is_generated "$path" || hand+=("$path")
    done <<<"$conflicted"
    if ((${#hand[@]})); then
      echo "restack-regen: these conflicted paths are not generated; resolve them by hand, then rerun:" >&2
      printf '  %s\n' "${hand[@]}" >&2
      exit 1
    fi
    # During a rebase "ours" is the branch being rebased onto, the upstream copy.
    while IFS= read -r path; do
      git checkout --ours -- "$path" 2>/dev/null || git rm -q -- "$path"
    done <<<"$conflicted"
    echo "restack-regen: regenerating after a stop on:"
    printf '  %s\n' "$conflicted"
    bash -c "$generate"
    git add -A
  fi
  GIT_EDITOR=true "$gt" continue || true
done
echo "restack-regen: gave up after 100 stops" >&2
exit 1
