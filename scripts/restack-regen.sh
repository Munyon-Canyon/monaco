#!/usr/bin/env bash
# Settles a stopped `gt restack` (or rebase) whose every conflicted path is generated: takes the
# upstream copy of each, runs the generators, stages the result and continues, until the rebase ends.
# With no rebase in progress it runs `gt restack` first. Any hand-written conflict stops it.
# GT, GENERATE and GENERATED_GLOBS let the test swap in fakes.
set -euo pipefail

gt=${GT:-gt}
generate=${GENERATE:-"cd apps/backend && go generate ./..."}
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
  rebasing || { echo "restack-regen: the restack finished"; exit 0; }
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
