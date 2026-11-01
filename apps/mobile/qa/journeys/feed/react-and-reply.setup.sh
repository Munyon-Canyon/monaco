#!/usr/bin/env bash
set -euo pipefail

run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
scenario="${1:?journey.py passes the scenario id}"
: "${MONACO_QA_HANDOFF:?journey.py sets MONACO_QA_HANDOFF}"
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

# The seed role feed-host is the same pooled dev user every run: Privy caps the app's users.
host_call() {
  local method="$1" route="$2" body="${3:-}" token
  token="$(_qa_monacoctl dev token --user new --pool feed-host --ttl 1h)"
  curl -fsS -X "$method" "$QA_API$route" -H "Authorization: Bearer $token" -H "Content-Type: application/json" \
    -H "Idempotency-Key: $(uuidgen | tr '[:upper:]' '[:lower:]')" ${body:+-d "$body"}
}

json_field() {
  python3 -c 'import json,sys; v=json.load(sys.stdin).get(sys.argv[1]); print("" if v is None else v)' "$1"
}

create_cabal() {
  host_call POST /v1/cabals \
    "{\"name\":\"$1\",\"join_mode\":\"request\",\"voter_mode\":\"all\",\"threshold\":\"majority\",\"proposal_expiry_seconds\":86400}" |
    json_field id
}

uuid7() {
  echo "encode(set_bit(set_bit(overlay(uuid_send(gen_random_uuid()) placing
    substring(int8send(floor(extract(epoch FROM clock_timestamp()) * 1000)::bigint) FROM 3) FROM 1 FOR 6),
    52, 1), 53, 1), 'hex')::uuid"
}

handoff() {
  python3 - "$MONACO_QA_HANDOFF" "$1" "$2" <<'PY'
import json, os, sys
path, key, value = sys.argv[1:4]
values = json.load(open(path)) if os.path.exists(path) and os.path.getsize(path) else {}
values[key] = value
json.dump(values, open(path, "w"))
PY
}

# Creating a proposal through the API needs a live Jupiter quote, so the rows are written the way
# governance.ProposeTrade writes them (see governance/vote.setup.sh).
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

# One reply by the host on a feed item, written as the comment row the API writes, with the item's count.
host_reply() {
  local item="$1" host="$2" body="$3"
  qa_sql -v item="$item" -v host="$host" -v body="$body" >/dev/null <<SQL
WITH reply AS (
  INSERT INTO feed_comments (id, feed_object_id, author_id, body, created_at)
  VALUES ($(uuid7), :'item', :'host', :'body', now())
  RETURNING id
)
UPDATE feed_objects SET comment_count = comment_count + 1 WHERE id = :'item'
SQL
}

# Only S1 seeds: S2 to S4 continue from the Feed post, its replies and the public cabal it wrote.
case "$scenario" in
  S1)
    ready_actor A
    host="$(host_call GET /v1/me | json_field id)"
    host_call PATCH /v1/me '{"display_name":"QA host"}' >/dev/null
    cabal_name="QA feed $run"
    cabal="$(create_cabal "$cabal_name")"
    request="$(qa_api A POST "/v1/cabals/$cabal/access-requests" 2>/dev/null | json_field id || true)"
    if [[ -n "${request:-}" ]]; then
      host_call POST "/v1/cabals/$cabal/access-requests/$request/decision" '{"decision":"approve"}' >/dev/null
    fi
    item="$(qa_sql -q -v cabal="$cabal" -v host="$host" -v name="$cabal_name" <<SQL
INSERT INTO feed_objects (id, kind, ref_type, ref_id, cabal_id, cabal_name, actor_id, symbol, title, payload,
  status, comment_count, created_at, updated_at)
VALUES ($(uuid7), 'trade', 'swaps', $(uuid7), :'cabal', :'name', :'host', 'TSLAx', 'sold TSLAx',
  '{"side":"sell","symbol":"TSLAx","actor_name":"QA host"}', 'completed', 0, now(), now())
RETURNING id
SQL
)"
    [[ -n "$item" ]] || { echo "the sale's feed item was not written" >&2; exit 1; }
    host_reply "$item" "$host" "QA root $run"
    handoff sale_item "$item"

    public_name="QA public $run"
    public_cabal="$(create_cabal "$public_name")"
    proposal="$(open_proposal "$public_cabal" "$host" "QA public proposal $run")"
    [[ -n "$proposal" ]] || { echo "no TSLAx row in assets, so no proposal was written" >&2; exit 1; }
    public_item=""
    for _ in $(seq 1 30); do
      public_item="$(qa_sql -v p="$proposal" <<<"SELECT id FROM feed_objects WHERE ref_type = 'proposals' AND ref_id = :'p'")"
      [[ -n "$public_item" ]] && break
      sleep 1
    done
    [[ -n "$public_item" ]] || { echo "the feed consumer did not write the public proposal's item" >&2; exit 1; }
    host_reply "$public_item" "$host" "QA public reply $run"
    handoff public_item "$public_item"
    echo "seeded: '$cabal_name' ($item) with A as a member, '$public_name' ($public_item) without A"
    ;;
  S2 | S3 | S4)
    echo "seeded in S1: nothing to add for $scenario"
    ;;
  *)
    echo "no setup for scenario $scenario" >&2
    exit 1
    ;;
esac
