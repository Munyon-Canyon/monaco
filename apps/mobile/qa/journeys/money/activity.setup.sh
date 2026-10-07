#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
scenario="${1:?journey.py passes the scenario id}"
# shellcheck source=/dev/null
source scripts/qa/seed.sh
qa_api_ready

field() {
  python3 -c 'import json,sys; v=json.load(sys.stdin).get(sys.argv[1]); print("" if v is None else v)' "$1"
}

ready_a() {
  local name
  qa_sql -v id="$(qa_user_id A)" >/dev/null \
    <<<"UPDATE users SET auth_state = 'ONBOARDING_COMPLETED', auth_state_changed_at = now() WHERE id = :'id' AND auth_state <> 'ONBOARDING_COMPLETED'"
  name="$(_qa_account A name)"
  if [[ "$(qa_api A GET /v1/me | field display_name)" != "$name" ]]; then
    qa_api A PATCH /v1/me "{\"display_name\":\"$name\"}" >/dev/null
  fi
}

create_cabal() {
  qa_api A POST /v1/cabals \
    "{\"name\":\"$1\",\"join_mode\":\"request\",\"voter_mode\":\"all\",\"threshold\":\"majority\",\"proposal_expiry_seconds\":86400}" |
    field id
}

ready_a
hand_off() {
  python3 - "${MONACO_QA_HANDOFF:?journey.py sets MONACO_QA_HANDOFF}" "$1" "$2" <<'PY'
import json, os, sys
path, key, value = sys.argv[1:]
values = json.load(open(path)) if os.path.exists(path) else {}
values[key] = value
json.dump(values, open(path, "w"))
PY
}

case "$scenario" in
  S1)
    txn="$(qa_sql -v user="$(qa_user_id A)" -v sig="QA$run" <<'SQL'
WITH t AS (
  INSERT INTO user_txns (id, user_id, kind, status, tx_signature, created_at)
  VALUES (gen_random_uuid(), :'user', 'deposit', 'settled', :'sig', now())
  RETURNING id
), e AS (
  INSERT INTO user_txn_entries (txn_id, seq, account, asset, amount)
  SELECT id, s.seq, s.account, 'EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v', s.amount
  FROM t, (VALUES (1, 'external', -1230000), (2, 'wallet', 1230000)) AS s (seq, account, amount)
)
SELECT id FROM t
SQL
)"
    hand_off deposit-txn "$txn"
    echo "seeded: a settled \$1.23 deposit $txn for A"
    ;;
  S2)
    create_cabal "QA activity $run" >/dev/null
    echo "seeded: A created the cabal 'QA activity $run'; the fund route (#608, #651) does not exist yet"
    ;;
  S3)
    echo "nothing to seed for S3: the withdraw route (#652) does not exist yet"
    ;;
  *)
    echo "no setup for scenario $scenario" >&2
    exit 1
    ;;
esac
