#!/usr/bin/env bash
# Browsing writes nothing. This checks the run read the seeded catalogue and created no proposal for it,
# then removes the seeded rows so the dev Stocks tab shows only the real catalogue. stocks/asset-detail
# runs it too.
set -euo pipefail

query="SELECT
  (SELECT string_agg(symbol || '=' || display_name || '=' || kind, ',' ORDER BY symbol) FROM assets
     WHERE symbol IN ('JRNYAx', 'JRNYPx', 'JRNYQx', 'JRNYZx')),
  (SELECT count(*) FROM proposals WHERE symbol IN ('JRNYAx', 'JRNYPx', 'JRNYQx', 'JRNYZx'))"
cleanup="DELETE FROM price_points WHERE mint LIKE 'QAJourney%';
DELETE FROM assets WHERE symbol IN ('JRNYAx', 'JRNYPx', 'JRNYQx', 'JRNYZx')"

psql_local() {
  scripts/with-dotenv-local.sh apps/mobile/qa/journeys/psql.sh -v ON_ERROR_STOP=1 -tA -F $'\t' "$@"
}

error_file="$(mktemp)"
trap 'rm -f "$error_file"' EXIT
set +e
row="$(printf '%s\n' "$query" | psql_local 2>"$error_file")"
status=$?
set -e

if [[ $status -eq 127 ]]; then
  grep -m1 "psql not found" "$error_file" >&2 || echo "psql not found (install it, or start Compose postgres)" >&2
  exit 2
fi
if [[ $status -ne 0 ]]; then
  awk '!/^with-dotenv-local:/ && !/injected env/ { print; exit }' "$error_file" >&2
  echo "database cannot be reached" >&2
  exit 2
fi

IFS=$'\t' read -r catalogue proposals <<<"$row"
expected="JRNYAx=Journey Alpha=equity,JRNYPx=Journey Private=pre_ipo,JRNYQx=Journey Private=pre_ipo,JRNYZx=Journey Zulu=equity"
if [[ "$catalogue" != "$expected" ]]; then
  echo "the seeded catalogue reads '$catalogue', not '$expected'"
  exit 1
fi
if [[ "$proposals" != "0" ]]; then
  echo "$proposals proposals name a seeded asset; looking at a stock must not propose anything"
  exit 1
fi
printf '%s\n' "$cleanup" | psql_local -q >/dev/null 2>"$error_file"
echo "the run read the seeded catalogue, proposed nothing, and the seeded rows are removed"
