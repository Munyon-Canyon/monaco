#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
scenario="${1:?journey.py passes the scenario id}"
# shellcheck disable=SC1091
source scripts/qa/seed.sh

# The cabal screen shows "Quiet in here." instead of Fund / Propose while the cabal has no proposal at all
# (CabalEmptyGate, #6993), so seed one closed buy proposal: expired a day ago, voted on by every voter.
uuid7() {
  echo "encode(set_bit(set_bit(overlay(uuid_send(gen_random_uuid()) placing
    substring(int8send(floor(extract(epoch FROM ${1:-clock_timestamp()}) * 1000)::bigint) FROM 3) FROM 1 FOR 6),
    52, 1), 53, 1), 'hex')::uuid"
}

seed_closed_proposal() {
  qa_sql -v cabal="$1" -v proposer="$(qa_user_id A)" >/dev/null <<SQL
WITH asset AS (
  SELECT symbol, mint, (5000000 * 10::numeric ^ decimals / coalesce(
    (SELECT price_micros FROM price_points WHERE price_points.mint = assets.mint ORDER BY ts DESC LIMIT 1),
    400000000))::bigint AS quote_out
  FROM assets WHERE symbol = 'TSLAx'
), voters AS (
  SELECT array_agg(user_id ORDER BY joined_at, user_id) AS ids
  FROM cabal_members WHERE cabal_id = :'cabal' AND can_vote
), proposal AS (
  INSERT INTO proposals (id, cabal_id, proposer_id, kind, symbol, mint, usdc_micros, token_amount, thesis,
    quote_out_amount, status, threshold, expires_at, created_at, updated_at)
  SELECT $(uuid7), :'cabal', :'proposer', 'buy', symbol, mint, 5000000, NULL, 'QA seed: an earlier proposal',
    quote_out, 'expired', (SELECT threshold FROM cabals WHERE id = :'cabal'), now() - interval '1 day',
    now() - interval '2 days', now() - interval '1 day'
  FROM asset
  RETURNING id, cabal_id, proposer_id, kind, symbol, mint, usdc_micros, quote_out_amount, expires_at, created_at
), voted AS (
  INSERT INTO proposal_voters (proposal_id, voter_id)
  SELECT proposal.id, voter FROM proposal, voters, unnest(voters.ids) AS voter
), created AS (
  INSERT INTO events (id, aggregate_type, aggregate_id, type, payload, actor_type, actor_id, created_at)
  SELECT $(uuid7), 'proposal', id, 'proposal.created',
    jsonb_build_object('v', 1, 'proposal_id', id, 'cabal_id', cabal_id, 'proposer_id', proposer_id, 'kind', kind,
      'symbol', symbol, 'mint', mint, 'usdc_micros', usdc_micros::text, 'quote_out_amount', quote_out_amount::text,
      'expires_at', expires_at, 'voter_count', cardinality(voters.ids)),
    'user', proposer_id::text, created_at
  FROM proposal, voters
), closed AS (
  INSERT INTO events (id, aggregate_type, aggregate_id, type, payload, actor_type, actor_id, created_at)
  SELECT $(uuid7 "clock_timestamp() + interval '1 millisecond'"), 'proposal', id, 'proposal.expired',
    jsonb_build_object('v', 1, 'proposal_id', id, 'cabal_id', cabal_id),
    'system', 'poller.governance.proposal_expiry', expires_at
  FROM proposal
)
SELECT id FROM proposal
SQL
}

if [[ "$scenario" != S1 ]]; then
  echo "no setup for scenario $scenario" >&2
  exit 1
fi

qa_api_ready
did="$(apps/mobile/qa/journeys/privy-user-id.sh A)"
qa_sql -v did="$did" >/dev/null \
  <<<"UPDATE users SET auth_state = 'ONBOARDING_COMPLETED', auth_state_changed_at = now() WHERE privy_user_id = :'did' AND auth_state <> 'ONBOARDING_COMPLETED'"
qa_seed_scenario cabal-with-confirmed-trade A
seed_closed_proposal "$CABAL_ID"
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
echo "seeded: A's cabal 'QA sell $run' holds $SYMBOL, priced for the next 16 minutes, with one closed proposal"
