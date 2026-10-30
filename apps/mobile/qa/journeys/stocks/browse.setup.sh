#!/usr/bin/env bash
# journey.py runs this before each scenario with the scenario id. It puts the journey's catalogue rows
# and their price samples in the local database (Preconditions P2 and P3), so no step taps to create them.
# For S1 it also takes actor A out of every cabal that holds a stock, so the Held step's empty state does
# not depend on which journey ran before (Precondition P5).
# stocks/asset-detail runs it too.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
# shellcheck source=/dev/null
source scripts/qa/seed.sh

leave_cabals_holding_stocks() {
  local id removed
  id="$(qa_user_id A)"
  removed="$(qa_sql -v id="$id" <<<"WITH gone AS (
    DELETE FROM cabal_members m
    WHERE m.user_id = :'id'
      AND EXISTS (SELECT 1 FROM cabal_positions p WHERE p.cabal_id = m.cabal_id AND p.units > 0)
    RETURNING 1
  ) SELECT count(*) FROM gone")"
  echo "$1: left $removed cabals that hold a stock as A"
}

if [[ "${1:-}" == S1 ]]; then
  leave_cabals_holding_stocks "$1"
fi

error_file="$(mktemp)"
trap 'rm -f "$error_file"' EXIT
samples="$(scripts/with-dotenv-local.sh apps/mobile/qa/journeys/psql.sh -v ON_ERROR_STOP=1 -tA -q \
  <apps/mobile/qa/journeys/stocks/browse.catalogue.sql 2>"$error_file")" || true

if [[ "$samples" != "128" ]]; then
  grep -v -e '^with-dotenv-local:' -e 'injected env' "$error_file" >&2 || true
  echo "${1:?usage: browse.setup.sh <scenario>}: could not seed the journey catalogue (got '${samples}' samples)" >&2
  exit 1
fi
echo "$1: seeded JRNYAx, JRNYPx, JRNYQx and JRNYZx with two price samples each, and JRNYAx with 120 five-minute samples more"
