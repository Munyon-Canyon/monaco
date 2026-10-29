#!/usr/bin/env bash
# journey.py runs this before each scenario. It completes actor B's onboarding, and before S2 it clears
# B's open proposals, makes a dev host create a cabal with B as a voter and one open proposal B has never voted on
# (P3), handing the test the cabal's name and the proposal's id. Before S4 it clears
# B's saved notification choices, so both categories start on.
set -euo pipefail

scenario="${1:?usage: notifications.setup.sh <scenario>}"
fail() {
  echo "$scenario: $*" >&2
  exit 1
}

if [[ "${NOTIFICATIONS_SETUP_ENV:-}" != 1 ]]; then
  NOTIFICATIONS_SETUP_ENV=1 exec scripts/with-dotenv-local.sh "$0" "$@" 2> >(grep -v -e '^with-dotenv-local:' -e 'injected env' >&2)
fi

: "${MONACO_QA_HANDOFF:?journey.py sets MONACO_QA_HANDOFF}"
run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
api="${MONACO_API_BASE_URL:-http://127.0.0.1:8080}"

sql() {
  apps/mobile/qa/journeys/psql.sh -v ON_ERROR_STOP=1 -tA "$@"
}

hand_off() {
  python3 - "$MONACO_QA_HANDOFF" "$1" "$2" <<'PY'
import json, os, sys
path, key, value = sys.argv[1:]
values = json.load(open(path)) if os.path.exists(path) else {}
values[key] = value
json.dump(values, open(path, "w"))
PY
}

call() {
  local method="$1" path="$2" bearer="$3" body="${4:-}" args
  args=(-fsS -X "$method" "$api$path" -H "Authorization: Bearer $bearer" -H 'Content-Type: application/json')
  [[ "$method" == GET ]] || args+=(-H "Idempotency-Key: $(uuidgen | tr '[:upper:]' '[:lower:]')")
  [[ -z "$body" ]] || args+=(-d "$body")
  curl "${args[@]}"
}

dev_token() {
  scripts/qa/monacoctl.sh dev token --user "$1" "${@:2}" --ttl 1h \
    2> >(grep -v -e '^with-dotenv-local:' -e 'injected env' >&2) || fail "monacoctl dev token --user $1 failed"
}

did="$(apps/mobile/qa/journeys/privy-user-id.sh B)"
row="$(sql -F ' ' -v did="$did" <<<"UPDATE users SET auth_state = 'ONBOARDING_COMPLETED', auth_state_changed_at = now()
  WHERE privy_user_id = :'did' RETURNING id, handle" | head -1)"
read -r user_id handle <<<"$row"
[[ -n "$user_id" && -n "$handle" ]] || fail "actor B has no users row or no handle. Run auth/sign-in as B first"
echo "$scenario: actor B (@$handle) is ONBOARDING_COMPLETED"
if [[ "$scenario" == S4 ]]; then
  sql -v uid="$user_id" <<<"DELETE FROM notification_preferences WHERE user_id = :'uid'" >/dev/null
  echo "$scenario: actor B's notification choices are cleared"
fi
[[ "$scenario" == S2 ]] || exit 0

token_b="$(dev_token "$user_id")"
sql -v uid="$user_id" >/dev/null <<<"UPDATE proposals SET status = 'expired', updated_at = now()
  WHERE status = 'open' AND id IN (SELECT proposal_id FROM proposal_voters WHERE voter_id = :'uid')"

uuid7() {
  echo "encode(set_bit(set_bit(overlay(uuid_send(gen_random_uuid()) placing
    substring(int8send(floor(extract(epoch FROM clock_timestamp()) * 1000)::bigint) FROM 3) FROM 1 FOR 6),
    52, 1), 53, 1), 'hex')::uuid"
}

host="$(dev_token new --pool push-host)"
call PATCH /v1/me "$host" '{"display_name":"QA push host"}' >/dev/null || fail "could not name the dev host"
host_id="$(call GET /v1/me "$host" | python3 -c 'import json, sys; print(json.load(sys.stdin)["id"])')"
name="QA push $run"
cabal="$(call POST /v1/cabals "$host" \
  "{\"name\":\"$name\",\"join_mode\":\"request\",\"voter_mode\":\"all\",\"threshold\":\"majority\",\"proposal_expiry_seconds\":86400}" |
  python3 -c 'import json, sys; print(json.load(sys.stdin)["id"])')" || fail "could not create '$name'"
request="$(call POST "/v1/cabals/$cabal/access-requests" "$token_b" | python3 -c 'import json, sys; print(json.load(sys.stdin)["id"])')" ||
  fail "B could not ask to join '$name'"
call POST "/v1/cabals/$cabal/access-requests/$request/decision" "$host" '{"decision":"approve"}' >/dev/null ||
  fail "could not admit @$handle"
proposal="$(sql -v cabal="$cabal" -v proposer="$host_id" <<SQL
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
  SELECT $(uuid7), :'cabal', :'proposer', 'buy', symbol, mint, 5000000, NULL, 'QA push thesis', quote_out, 'open',
    (SELECT threshold FROM cabals WHERE id = :'cabal'), now() + interval '1 day', now(), now()
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
)
SELECT id FROM proposal
SQL
)" || fail "could not open a proposal in '$name'"
hand_off cabalName "$name"
hand_off proposalID "$proposal"
echo "$scenario: '$name' has B as a voter and open proposal $proposal"
