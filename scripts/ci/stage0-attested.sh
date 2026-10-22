#!/usr/bin/env bash
# Stage 0 attestation, docs/architecture/ci.md#check-stages. Writes skip=true to $GITHUB_OUTPUT only when the head
# carries a stage0/ok status for its own tree and no path filter wants stage 1. Anything else, including an API failure,
# writes skip=false, so the full stage 1 runs.
set -euo pipefail

: "${HEAD_SHA:?}" "${GITHUB_REPOSITORY:?}" "${GITHUB_OUTPUT:?}"

decide() {
  echo "skip=$1" >>"$GITHUB_OUTPUT"
  echo "stage 0 attestation: skip=$1 ($2)"
  exit 0
}

for filter in IOS MOBILE_CORE MOBILE_TESTS JOURNEYS WEB WORKFLOWS UNKNOWN; do
  if [[ "${!filter:-}" != false ]]; then
    decide false "$filter is ${!filter:-unset}, so stage 1 runs in full"
  fi
done

tree="$(git rev-parse "$HEAD_SHA^{tree}")" || decide false "no tree for $HEAD_SHA"
if ! description="$(gh api "repos/$GITHUB_REPOSITORY/commits/$HEAD_SHA/statuses" \
  --jq '[.[] | select(.context == "stage0/ok" and .state == "success")] | first | .description // ""')"; then
  echo "::warning::stage 0 attestation lookup failed, so the full stage 1 runs"
  decide false "status lookup failed"
fi
if [[ "$description" == "tree $tree" ]]; then
  decide true "stage0/ok on $HEAD_SHA names tree $tree and every filter is false"
fi
decide false "no stage0/ok status for tree $tree on $HEAD_SHA (newest says '${description:-none}')"
