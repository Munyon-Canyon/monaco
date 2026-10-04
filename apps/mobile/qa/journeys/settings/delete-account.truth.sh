#!/usr/bin/env bash
set -euo pipefail

: "${MONACO_QA_HANDOFF:?journey.py sets MONACO_QA_HANDOFF}"

handed() {
  python3 -c 'import json, sys; print(json.load(open(sys.argv[1])).get(sys.argv[2], ""))' "$MONACO_QA_HANDOFF" "$1"
}

row() {
  printf '%s\n' "SELECT account_status, deleted_at IS NOT NULL, display_name = '', handle IS NOT NULL FROM users WHERE id = :'id'" |
    scripts/with-dotenv-local.sh apps/mobile/qa/journeys/psql.sh -v ON_ERROR_STOP=1 -v id="$1" -tA -F ' ' \
      2> >(grep -v -e '^with-dotenv-local:' -e 'injected env' >&2)
}

status=0
for scenario in S1 S2 S3; do
  id="$(handed "devUser$scenario")"
  [[ -n "$id" ]] || continue
  got="$(row "$id")" || { echo "database cannot be reached" >&2; exit 2; }
  if [[ "$scenario" == S2 ]]; then
    want="deleted t t t"
  else
    want="$(awk '{ print $1, $2 }' <<<"$got")"
    [[ "$want" == "active f" ]] || { echo "$scenario's dev user $id was deleted or changed: '$got'"; status=1; }
    continue
  fi
  if [[ "$got" != "$want" ]]; then
    echo "S2's dev user $id reads '$got' (account_status, deleted, name cleared, handle kept), not '$want'"
    status=1
  else
    echo "S2's dev user $id is deleted, scrubbed, and keeps its handle"
  fi
done
exit "$status"
