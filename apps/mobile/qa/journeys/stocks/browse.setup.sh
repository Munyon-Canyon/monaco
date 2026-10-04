#!/usr/bin/env bash
# journey.py runs this before each scenario with the scenario id. It puts the journey's catalogue rows
# and their price samples in the local database (Preconditions P2 and P3), so no step taps to create them.
# stocks/asset-detail runs it too.
set -euo pipefail

error_file="$(mktemp)"
trap 'rm -f "$error_file"' EXIT
samples="$(scripts/with-dotenv-local.sh apps/mobile/qa/journeys/psql.sh -v ON_ERROR_STOP=1 -tA -q \
  <apps/mobile/qa/journeys/stocks/browse.catalogue.sql 2>"$error_file")" || true

if [[ "$samples" != "8" ]]; then
  grep -v -e '^with-dotenv-local:' -e 'injected env' "$error_file" >&2 || true
  echo "${1:?usage: browse.setup.sh <scenario>}: could not seed the journey catalogue (got '${samples}' samples)" >&2
  exit 1
fi
echo "$1: seeded JRNYAx, JRNYPx, JRNYQx and JRNYZx with two price samples each"
