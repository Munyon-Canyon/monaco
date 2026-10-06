#!/usr/bin/env bash
# The chart read price samples (flow 18): the latest `price_points` row for the seeded JRNYAx mint is the price
# the screen shows and is recent. The shared browse check then verifies the catalogue and removes the seeded rows.
set -euo pipefail

query="SELECT p.price_micros, p.ts > now() - interval '30 minutes'
FROM price_points p JOIN assets a ON a.mint = p.mint
WHERE a.symbol = 'JRNYAx' AND p.source = 'qa-journey' ORDER BY p.ts DESC LIMIT 1"

error_file="$(mktemp)"
trap 'rm -f "$error_file"' EXIT
set +e
row="$(printf '%s\n' "$query" | scripts/with-dotenv-local.sh apps/mobile/qa/journeys/psql.sh -v ON_ERROR_STOP=1 -tA -F $'\t' 2>"$error_file")"
status=$?
set -e
if [[ $status -ne 0 ]]; then
  awk '!/^with-dotenv-local:/ && !/injected env/ { print; exit }' "$error_file" >&2
  echo "database cannot be reached" >&2
  exit 2
fi
if [[ "$row" != $'123450000\tt' ]]; then
  echo "the latest price_points row for JRNYAx reads '$row', not a 123450000 sample from the last 30 minutes"
  exit 1
fi
exec apps/mobile/qa/journeys/stocks/browse.truth.sh
