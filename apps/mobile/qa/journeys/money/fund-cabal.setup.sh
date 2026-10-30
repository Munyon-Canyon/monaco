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
case "$scenario" in
  S1 | S2)
    name="QA fund $run"
    if [[ "$(qa_sql -v name="$name" <<<"SELECT count(*) FROM cabals WHERE name = :'name'")" == 0 ]]; then
      create_cabal "$name" >/dev/null
    fi
    # The journey backend runs no price poller, so a stale price makes the fund preview answer prices_stale.
    # Samples every 4 minutes up to 16 minutes out keep fresh every asset the cabal and A hold; each repeats
    # the newest real sample so the outlier filter accepts it, and an asset with no sample is left alone.
    qa_sql -v name="$name" -v a="$(qa_user_id A)" >/dev/null <<'SQL'
INSERT INTO price_points (mint, ts, price_micros, source)
SELECT held.mint, ts, last.price_micros, 'qa-journey'
FROM (
  SELECT DISTINCT p.asset AS mint FROM cabal_positions p
  WHERE p.units > 0 AND p.cabal_id IN (
    SELECT id FROM cabals WHERE name = :'name'
    UNION SELECT cabal_id FROM cabal_members WHERE user_id = :'a')
) AS held
CROSS JOIN LATERAL (
  SELECT price_micros FROM price_points
  WHERE mint = held.mint AND ts <= now() ORDER BY ts DESC LIMIT 1
) AS last
CROSS JOIN generate_series(now(), now() + interval '16 minutes', interval '4 minutes') AS ts
ON CONFLICT (mint, ts) DO NOTHING
SQL
    echo "seeded: A created the cabal '$name', and the prices of what it holds are fresh for 16 minutes"
    ;;
  *)
    echo "no setup for scenario $scenario" >&2
    exit 1
    ;;
esac
