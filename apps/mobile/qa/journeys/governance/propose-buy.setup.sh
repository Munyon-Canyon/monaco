#!/usr/bin/env bash
# journey.py runs this before each scenario of docs/journeys/governance/propose-buy.md with the scenario id.
# S1 puts A and B on the cabal that monacoctl dev seed-scenario cabal-with-funded-pot makes: A created it,
# B joined, and the ledger credits A $2.00 and B $1.00. S2 continues from the proposal S1 sent.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
scenario="${1:?journey.py passes the scenario id}"
handoff="${MONACO_QA_HANDOFF:?journey.py sets MONACO_QA_HANDOFF}"
# shellcheck disable=SC1091
source scripts/qa/seed.sh

case "$scenario" in
  S1) ;;
  S2)
    grep -q cabalName "$handoff" || {
      echo "no cabalName in the hand-off: S2 continues from S1" >&2
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
  name="$(awk -F '\t' -v actor="$actor" '$1 == actor { print $2 }' apps/mobile/qa/journeys/accounts.tsv)"
  qa_sql -v did="$did" >/dev/null \
    <<<"UPDATE users SET auth_state = 'ONBOARDING_COMPLETED', auth_state_changed_at = now() WHERE privy_user_id = :'did' AND auth_state <> 'ONBOARDING_COMPLETED'"
  qa_api "$actor" PATCH /v1/me "{\"display_name\":\"$name\"}" >/dev/null
  if [[ -z "$(qa_api "$actor" GET /v1/me | python3 -c 'import json,sys; print(json.load(sys.stdin).get("handle") or "")')" ]]; then
    qa_api "$actor" PUT /v1/me/handle "{\"handle\":\"qa_$(tr '[:upper:]' '[:lower:]' <<<"$actor")\"}" >/dev/null
  fi
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

qa_seed_scenario cabal-with-funded-pot A B
python3 - "$handoff" "$CABAL_NAME" <<'PY'
import json, os, sys
path, name = sys.argv[1:]
values = json.load(open(path)) if os.path.exists(path) and os.path.getsize(path) else {}
values["cabalName"] = name
json.dump(values, open(path, "w"))
PY
echo "seeded S1: expired $expired open proposals of A or B, cabal '$CABAL_NAME' ($CABAL_ID) with a \$3.00 pot"
