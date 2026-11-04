#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
scenario="${1:?journey.py passes the scenario id}"
api="${MONACO_API_BASE_URL:-http://127.0.0.1:8080}"
accounts="${QA_ACCOUNTS_FILE:-apps/mobile/qa/journeys/accounts.tsv}"

if ! curl -fsS "$api/healthz" >/dev/null; then
  echo "the backend does not answer $api/healthz: just run backend first" >&2
  exit 1
fi

column() {
  awk -F '\t' -v actor="$1" -v want="$2" '
    $1 == "actor" { for (i = 1; i <= NF; i++) if ($i == want) col = i; next }
    $1 == actor && col { print $col; exit }
  ' "$accounts"
}

user_id() {
  local did
  did="$(column "$1" privy_user_id)"
  if [[ -z "$did" ]]; then
    echo "actor $1 has no privy_user_id in $accounts: sign in as $1 once, then add the did" >&2
    exit 1
  fi
  local id
  id="$(docker compose exec -T postgres sh -c \
    'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -tA -v did="$1"' sh "$did" \
    <<<"SELECT id FROM users WHERE privy_user_id = :'did'")"
  if [[ -z "$id" ]]; then
    echo "actor $1 ($did) has no users row: sign in as $1 once first" >&2
    exit 1
  fi
  echo "$id"
}

token() {
  local error_file out
  error_file="$(mktemp)"
  if ! out="$(scripts/qa/monacoctl.sh dev token --user "$1" "${@:2}" --ttl 1h 2>"$error_file")"; then
    grep -v -e '^with-dotenv-local:' -e 'injected env' "$error_file" >&2 || true
    echo "monacoctl dev token --user $1 failed" >&2
    rm -f "$error_file"
    exit 1
  fi
  rm -f "$error_file"
  echo "$out"
}

call() {
  local method="$1" path="$2" bearer="$3" body="${4:-}"
  local args=(-fsS -X "$method" "$api$path" -H "Authorization: Bearer $bearer" -H "Content-Type: application/json")
  if [[ "$method" != GET ]]; then
    args+=(-H "Idempotency-Key: $(uuidgen | tr '[:upper:]' '[:lower:]')")
  fi
  if [[ -n "$body" ]]; then
    args+=(-d "$body")
  fi
  curl "${args[@]}"
}

field() {
  python3 -c 'import json,sys; v=json.load(sys.stdin).get(sys.argv[1]); print("" if v is None else v)' "$1"
}

ready_actor() {
  local actor="$1" handle="$2" bearer="$3" me
  me="$(call GET /v1/me "$bearer")"
  if [[ -z "$(field display_name <<<"$me")" ]]; then
    call PATCH /v1/me "$bearer" "{\"display_name\":\"$(column "$actor" name)\"}" >/dev/null
  fi
  if [[ -z "$(field handle <<<"$me")" ]]; then
    call PUT /v1/me/handle "$bearer" "{\"handle\":\"$handle\"}" >/dev/null
  fi
  call GET /v1/me "$bearer" | field handle
}

decline_all() {
  local bearer="$1"
  call GET /v1/me/cabal-invites "$bearer" | python3 -c '
import json, sys
for invite in json.load(sys.stdin):
    print(invite["cabal"]["id"], invite["request_id"])
' | while read -r cabal request; do
    call POST "/v1/cabals/$cabal/access-requests/$request/decision" "$bearer" '{"decision":"deny"}' >/dev/null
  done
}

uuid7() {
  echo "encode(set_bit(set_bit(overlay(uuid_send(gen_random_uuid()) placing
    substring(int8send(floor(extract(epoch FROM clock_timestamp()) * 1000)::bigint) FROM 3) FROM 1 FOR 6),
    52, 1), 53, 1), 'hex')::uuid"
}

# Creating a proposal through the API needs a live Jupiter quote, so the rows are written the way
# governance.ProposeTrade writes them (see feed/react-and-reply.setup.sh).
open_proposal() {
  local cabal="$1" proposer="$2" thesis="$3"
  scripts/with-dotenv-local.sh apps/mobile/qa/journeys/psql.sh -v ON_ERROR_STOP=1 -tA -q \
    -v cabal="$cabal" -v proposer="$proposer" -v thesis="$thesis" \
    2> >(grep -v -e '^with-dotenv-local:' -e 'injected env' >&2) >/dev/null <<SQL
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

for actor in A B; do
  printf '%s\n' "UPDATE users SET auth_state = 'ONBOARDING_COMPLETED', auth_state_changed_at = now() WHERE privy_user_id = :'did' AND auth_state <> 'ONBOARDING_COMPLETED'" |
    scripts/with-dotenv-local.sh apps/mobile/qa/journeys/psql.sh -v ON_ERROR_STOP=1 -v did="$(column "$actor" privy_user_id)" -tA \
      2> >(grep -v -e '^with-dotenv-local:' -e 'injected env' >&2) >/dev/null
done
token_a="$(token "$(user_id A)")"
token_b="$(token "$(user_id B)")"
handle_a="$(ready_actor A qa_a "$token_a")"
handle_b="$(ready_actor B qa_b "$token_b")"
if [[ "$handle_b" != qa_b ]]; then
  echo "actor B's handle is @$handle_b; the journey invites @qa_b" >&2
  exit 1
fi
decline_all "$token_a"
decline_all "$token_b"

case "$scenario" in
  S1)
    host_token="$(token new --pool invite-host)"
    call PATCH /v1/me "$host_token" '{"display_name":"QA host"}' >/dev/null
    name="QA pot $(date +%H%M%S)"
    cabal="$(call POST /v1/cabals "$host_token" \
      "{\"name\":\"$name\",\"join_mode\":\"request\",\"voter_mode\":\"all\",\"threshold\":\"majority\",\"proposal_expiry_seconds\":86400}" |
      field id)"
    host_id="$(call GET /v1/me "$host_token" | field id)"
    open_proposal "$cabal" "$host_id" "QA invite proposal $run"
    call POST "/v1/cabals/$cabal/invites" "$host_token" "{\"handle\":\"$handle_a\"}" >/dev/null
    echo "seeded: $name, request, one open TSLAx proposal, invites @$handle_a; @$handle_b has no pending invites"
    ;;
  S2)
    printf '%s\n' "DELETE FROM cabal_members m USING cabals c WHERE m.cabal_id = c.id AND m.user_id = :'id' AND c.name LIKE 'QA mine %'" |
      scripts/with-dotenv-local.sh apps/mobile/qa/journeys/psql.sh -v ON_ERROR_STOP=1 -v id="$(user_id A)" -tA \
        2> >(grep -v -e '^with-dotenv-local:' -e 'injected env' >&2) >/dev/null
    name="QA mine $run"
    call POST /v1/cabals "$token_a" \
      "{\"name\":\"$name\",\"join_mode\":\"request\",\"voter_mode\":\"all\",\"threshold\":\"majority\",\"proposal_expiry_seconds\":86400}" >/dev/null
    echo "seeded: $name, created by @$handle_a; @$handle_b has no pending invites"
    ;;
  *)
    echo "no setup for scenario $scenario" >&2
    exit 1
    ;;
esac
