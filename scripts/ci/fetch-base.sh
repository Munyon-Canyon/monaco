#!/usr/bin/env bash
# Print one commit SHA, the pull request base, on stdout. Attempts go to stderr.
# Order: PUSH_SHA's parent when no base is recorded, then BASE_SHA, then a BASE_REF that is
# not a gtmq_ queue branch, then FEATURE_BRANCH when no base was recorded at all.
# A recorded base that cannot be fetched fails, since the trunk tip may be newer than the commit under test.
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

if [[ -z "${BASE_SHA:-}" && -z "${BASE_REF:-}" && -n "${PUSH_SHA:-}" ]]; then
  note_try "${PUSH_SHA}^"
  if ! git fetch --no-tags --depth=2 origin "$PUSH_SHA"; then
    echo "fetch-base: failed to fetch push commit $PUSH_SHA" >&2
    exit 1
  fi
  if ! parent="$(git rev-parse --verify "${PUSH_SHA}^")"; then
    echo "fetch-base: push commit $PUSH_SHA has no parent" >&2
    exit 1
  fi
  echo "fetch-base: push $PUSH_SHA compares against its parent $parent" >&2
  printf '%s\n' "$parent"
  exit 0
fi

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

if [[ -n "$tried" ]]; then
  echo "fetch-base: failed, tried ${tried}" >&2
  exit 1
fi

echo "no base recorded, falling back to ${FEATURE_BRANCH}" >&2
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
