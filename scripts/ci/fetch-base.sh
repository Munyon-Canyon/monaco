#!/usr/bin/env bash
# Print one commit SHA, the pull request base, on stdout. Attempts go to stderr.
# Order: BASE_SHA, then a BASE_REF that is not a gtmq_ queue branch, then FEATURE_BRANCH.
set -euo pipefail

: "${FEATURE_BRANCH:?FEATURE_BRANCH is required}"

tried=""
note_try() {
  if [[ -z "$tried" ]]; then
    tried="$1"
  else
    tried="$tried $1"
  fi
  echo "fetch-base: trying $1" >&2
}

fetch_ref() {
  # shellcheck disable=SC2086 # DEPTH is one integer; empty means a full fetch
  git fetch --no-tags ${DEPTH:+--depth=$DEPTH} origin "$1"
}

if [[ -n "${BASE_SHA:-}" ]]; then
  note_try "$BASE_SHA"
  if fetch_ref "$BASE_SHA"; then
    printf '%s\n' "$BASE_SHA"
    exit 0
  fi
fi

if [[ -n "${BASE_REF:-}" && "$BASE_REF" != gtmq_* ]]; then
  note_try "$BASE_REF"
  if fetch_ref "$BASE_REF"; then
    git rev-parse FETCH_HEAD
    exit 0
  fi
fi

# Built at runtime: scripts/tool_manifest_test.go splits source on a literal semicolon.
sep="$(printf '\073')"
echo "base ${BASE_REF:-${BASE_SHA:-}} not fetchable${sep} falling back to ${FEATURE_BRANCH}" >&2
note_try "$FEATURE_BRANCH"
if fetch_ref "$FEATURE_BRANCH"; then
  if merge_base="$(git merge-base FETCH_HEAD HEAD 2>/dev/null)"; then
    printf '%s\n' "$merge_base"
  else
    git rev-parse FETCH_HEAD
  fi
  exit 0
fi

echo "fetch-base: failed, tried ${tried}" >&2
exit 1
