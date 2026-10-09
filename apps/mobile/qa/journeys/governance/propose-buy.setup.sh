#!/usr/bin/env bash
# journey.py runs this before each scenario of docs/journeys/governance/propose-buy.md with the scenario id.
# S1 makes the cabal through the API: A creates it and B joins it, with an empty pot. The app funds it in S1.
# Every later scenario continues from the one before.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
scenario="${1:?journey.py passes the scenario id}"
handoff="${MONACO_QA_HANDOFF:?journey.py sets MONACO_QA_HANDOFF}"
# shellcheck disable=SC1091
source scripts/qa/seed.sh

case "$scenario" in
  S1) ;;
  S2 | S3 | S4 | S5)
    grep -q cabalName "$handoff" || {
      echo "no cabalName in the hand-off: $scenario continues from S1" >&2
      exit 1
    }
    exit 0
    ;;
  *)
    echo "no setup for scenario $scenario" >&2
    exit 1
    ;;
esac

qa_api_ready
for actor in A B; do
  did="$(apps/mobile/qa/journeys/privy-user-id.sh "$actor")"
  name="$(awk -F '\t' -v actor="$actor" '$1 == actor { print $2 }' "${QA_ACCOUNTS_FILE:-apps/mobile/qa/journeys/accounts.tsv}")"
  qa_sql -v did="$did" >/dev/null \
    <<<"UPDATE users SET auth_state = 'ONBOARDING_COMPLETED', auth_state_changed_at = now() WHERE privy_user_id = :'did' AND auth_state <> 'ONBOARDING_COMPLETED'"
  qa_api "$actor" PATCH /v1/me "{\"display_name\":\"$name\"}" >/dev/null
done

expired="$(qa_sql -v a="$(qa_user_id A)" -v b="$(qa_user_id B)" <<'SQL'
WITH due AS (
  UPDATE proposals SET status = 'expired', updated_at = now()
  WHERE status = 'open'
    AND id IN (SELECT proposal_id FROM proposal_voters WHERE voter_id IN (:'a', :'b'))
  RETURNING 1
)
SELECT count(*) FROM due
SQL
)"

name="QA buy ${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
id="$(qa_api A POST /v1/cabals \
  "{\"name\":\"$name\",\"join_mode\":\"request\",\"voter_mode\":\"all\",\"threshold\":\"majority\",\"proposal_expiry_seconds\":86400}" |
  python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')"
qa_admit B A "$id"
python3 - "$handoff" "$name" <<'PY'
import json, os, sys
path, name = sys.argv[1:]
values = json.load(open(path)) if os.path.exists(path) and os.path.getsize(path) else {}
values["cabalName"] = name
json.dump(values, open(path, "w"))
PY
echo "seeded S1: expired $expired open proposals of A or B, A created the cabal '$name' ($id) and B joined it"
