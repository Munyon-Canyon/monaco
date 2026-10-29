#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=/dev/null
source "$(git rev-parse --show-toplevel)/scripts/qa/seed.sh"
handoff="${MONACO_QA_HANDOFF:?journey.py sets MONACO_QA_HANDOFF}"

a="$(qa_user_id A)" || exit 2
unranked="$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1])).get("unrankedID", ""))' "$handoff")"
got="$(qa_sql -v a="$a" -v u="$unranked" <<<"SELECT
  (SELECT count(*) FROM call_voters WHERE user_id = :'a')::text || ' ' ||
  (SELECT count(*) FROM call_voters WHERE user_id = NULLIF(:'u', '')::uuid)::text")" ||
  { echo "database cannot be reached" >&2; exit 2; }
if [[ "$got" != "7 2" ]]; then
  echo "call_voters rows of A and of the unranked member: got '$got', want '7 2': reading the boards wrote or lost votes"
  exit 1
fi
echo "ok: A still has seven votes and the unranked member two"
