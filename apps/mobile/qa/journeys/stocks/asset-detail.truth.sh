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

# S5: this run's setup bought JRNYAx through the product path. The fake chain balance is not checked: the fakes
# have no read endpoint for it.
cabal_name="QA holds JRNYAx ${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
s5_query="SELECT
  (SELECT count(*) FROM cabal_positions p JOIN cabals c ON c.id = p.cabal_id JOIN assets a ON a.mint = p.asset
     WHERE c.name = :'name' AND a.symbol = 'JRNYAx' AND p.units > 0),
  (SELECT count(*) FROM proposals r JOIN cabals c ON c.id = r.cabal_id
     WHERE c.name = :'name' AND r.symbol IN ('JRNYAx', 'JRNYPx', 'JRNYQx', 'JRNYZx'))"
set +e
s5="$(printf '%s\n' "$s5_query" | scripts/with-dotenv-local.sh apps/mobile/qa/journeys/psql.sh -v ON_ERROR_STOP=1 -tA -F $'\t' -v name="$cabal_name" 2>"$error_file")"
status=$?
set -e
if [[ $status -ne 0 ]]; then
  awk '!/^with-dotenv-local:/ && !/injected env/ { print; exit }' "$error_file" >&2
  echo "database cannot be reached" >&2
  exit 2
fi
if [[ "$s5" != $'1\t1' ]]; then
  echo "cabal '$cabal_name' has (JRNYAx positions, seeded-asset proposals) '$s5', not '1<tab>1': S5's setup did not buy JRNYAx once"
  exit 1
fi
QA_ALLOWED_CABAL_NAME="$cabal_name" exec apps/mobile/qa/journeys/stocks/browse.truth.sh
