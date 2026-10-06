#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
scenario="${1:?journey.py passes the scenario id}"
# shellcheck disable=SC1091
source scripts/qa/seed.sh

if [[ "$scenario" != S1 ]]; then
  echo "no setup for scenario $scenario" >&2
  exit 1
fi

qa_api_ready
did="$(apps/mobile/qa/journeys/privy-user-id.sh A)"
qa_sql -v did="$did" >/dev/null \
  <<<"UPDATE users SET auth_state = 'ONBOARDING_COMPLETED', auth_state_changed_at = now() WHERE privy_user_id = :'did' AND auth_state <> 'ONBOARDING_COMPLETED'"
qa_seed_scenario cabal-with-confirmed-trade A
qa_sql -v cabal="$CABAL_ID" -v name="QA sell $run" >/dev/null \
  <<<"UPDATE cabals SET name = :'name' WHERE id = :'cabal'"
# The pot refuses a holding whose newest price is older than 5 minutes (price_unavailable), and the
# journey backend runs no price poller. Samples every 4 minutes up to 16 minutes out keep the price fresh
# for the whole run; the price repeats the newest real sample so the outlier filter accepts it.
qa_sql -v mint="$ASSET_MINT" >/dev/null <<'SQL'
INSERT INTO price_points (mint, ts, price_micros, source)
SELECT :'mint', ts, coalesce(
    (SELECT price_micros FROM price_points WHERE mint = :'mint' AND ts <= now() ORDER BY ts DESC LIMIT 1),
    230000000), 'qa-journey'
FROM generate_series(now(), now() + interval '16 minutes', interval '4 minutes') AS ts
ON CONFLICT (mint, ts) DO NOTHING
SQL
echo "seeded: A's cabal 'QA sell $run' holds $SYMBOL, priced for the next 16 minutes"
