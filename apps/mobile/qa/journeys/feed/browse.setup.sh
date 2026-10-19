#!/usr/bin/env bash
set -euo pipefail

run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
scenario="${1:?journey.py passes the scenario id}"
# shellcheck source=/dev/null
source "$(git rev-parse --show-toplevel)/scripts/qa/seed.sh"
qa_api_ready

ready_actor() {
  local actor="$1" did name
  did="$(_qa_account "$actor" privy_user_id)"
  qa_sql -v did="$did" >/dev/null \
    <<<"UPDATE users SET auth_state = 'ONBOARDING_COMPLETED', auth_state_changed_at = now() WHERE privy_user_id = :'did' AND auth_state <> 'ONBOARDING_COMPLETED'"
  name="$(_qa_account "$actor" name)"
  qa_api "$actor" PATCH /v1/me "{\"display_name\":\"$name\"}" >/dev/null
}

create_cabal() {
  qa_api "$1" POST /v1/cabals \
    "{\"name\":\"$2\",\"join_mode\":\"request\",\"voter_mode\":\"all\",\"threshold\":\"majority\",\"proposal_expiry_seconds\":86400}" |
    python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])'
}

unfollow_all() {
  local actor="$1" me followees followee
  me="$(qa_user_id "$actor")"
  followees="$(qa_sql -v me="$me" <<<"SELECT followee_id FROM follows WHERE follower_id = :'me' AND deleted_at IS NULL")"
  for followee in $followees; do
    qa_api "$actor" DELETE "/v1/users/$followee/follow" >/dev/null
  done
}

uuid7() {
  echo "encode(set_bit(set_bit(overlay(uuid_send(gen_random_uuid()) placing
    substring(int8send(floor(extract(epoch FROM clock_timestamp()) * 1000)::bigint) FROM 3) FROM 1 FOR 6),
    52, 1), 53, 1), 'hex')::uuid"
}

open_proposal() {
  local cabal="$1" proposer="$2" thesis="$3"
  qa_sql -v cabal="$cabal" -v proposer="$proposer" -v thesis="$thesis" <<SQL
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
  SELECT $(uuid7), :'cabal', :'proposer', 'buy', symbol, mint, 5000000, NULL, :'thesis',
    quote_out, 'open', (SELECT threshold FROM cabals WHERE id = :'cabal'), now() + interval '86400 seconds',
    now(), now()
  FROM asset
  RETURNING id, cabal_id, proposer_id, kind, symbol, mint, usdc_micros, quote_out_amount, status, expires_at, created_at
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
)
SELECT id FROM proposal
SQL
}

# The shape of the testkit scenario feed-two-cabals, seeded as the QA accounts. Only S1 creates the cabals; the
# later scenarios continue from them.
case "$scenario" in
  S1)
    ready_actor A
    ready_actor B
    unfollow_all A
    cabal="$(create_cabal B "QA feed $run")"
    qa_admit A B "$cabal"
    create_cabal A "QA own $run" >/dev/null
    echo "seeded: B created 'QA feed $run' and A joined it, A created 'QA own $run', A follows nobody"
    ;;
  S2)
    unfollow_all A
    echo "seeded: A follows nobody"
    ;;
  S3)
    cabal="$(qa_sql -v name="QA feed $run" <<<"SELECT id FROM cabals WHERE name = :'name' ORDER BY created_at DESC LIMIT 1")"
    [[ -n "$cabal" ]] || { echo "no cabal 'QA feed $run': S1 did not seed it" >&2; exit 1; }
    proposal="$(open_proposal "$cabal" "$(qa_user_id B)" "QA proposal $run")"
    [[ -n "$proposal" ]] || { echo "no TSLAx row in assets, so no proposal was written" >&2; exit 1; }
    echo "seeded: B proposed a buy in 'QA feed $run' ($proposal) with the thesis 'QA proposal $run'"
    ;;
  *)
    echo "no setup for scenario $scenario" >&2
    exit 1
    ;;
esac
