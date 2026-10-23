#!/usr/bin/env bash
# Print the packages stage 1 tests on a pull request, one ./dir per line: the packages monacoctl ci affected
# selects for the diff from BASE, plus the agent tooling it excludes (stage 0 skips that, stage 1 tests it).
# STAGE1_FULL=true prints nothing and exits 0; the caller then leaves TEST_BACKEND_PACKAGES unset, which tests
# every package.
set -euo pipefail

if [[ "${STAGE1_FULL:-}" == true ]]; then
  exit 0
fi
: "${BASE:?BASE is required}"

cd "$(dirname "$0")/../../apps/backend"
{
  go run ./cmd/monacoctl ci affected --base "$BASE"
  go run ./cmd/monacoctl ci affected --base "$BASE" --excluded
} | LC_ALL=C sort -u
