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
seed_closed_proposal "$id"
python3 - "$handoff" "$name" <<'PY'
import json, os, sys
path, name = sys.argv[1:]
values = json.load(open(path)) if os.path.exists(path) and os.path.getsize(path) else {}
values["cabalName"] = name
json.dump(values, open(path, "w"))
PY
echo "seeded S1: expired $expired open proposals of A or B, A created the cabal '$name' ($id) B joined it and the cabal has one closed proposal"
