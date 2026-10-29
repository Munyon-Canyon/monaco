#!/usr/bin/env bash
# journey.py runs this before scenario S1 of docs/journeys/home/activity.md with the scenario id.
# It gives A one Waiting on you item (an open buy A has not voted on), one Replies and mentions item (the host's
# reply to A's comment) and one Results item (a buy A voted Yes on that the host's Yes passed), in a new cabal
# from the pooled "QA catchup host" user. The proposal rows are written the way governance.ProposeTrade writes
# them, because the propose route needs a Jupiter route and pot funds.
set -euo pipefail

scenario="${1:?usage: activity.setup.sh <S1>}"
[[ "$scenario" == S1 ]] || { echo "activity.setup.sh: unknown scenario '$scenario'" >&2 && exit 1; }

cd "$(git rev-parse --show-toplevel)"
if [[ "${ACTIVITY_SETUP_ENV:-}" != 1 ]]; then
  ACTIVITY_SETUP_ENV=1 exec scripts/with-dotenv-local.sh "$0" "$@" 2> >(grep -v -e '^with-dotenv-local:' -e 'injected env' >&2)
fi

run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
handoff="${MONACO_QA_HANDOFF:?journey.py sets MONACO_QA_HANDOFF}"
api="${MONACO_API_BASE_URL:-http://127.0.0.1:8080}"
accounts="${QA_ACCOUNTS_FILE:-apps/mobile/qa/journeys/accounts.tsv}"

fail() {
  echo "activity.setup.sh $scenario: $*" >&2
  exit 1
}

curl -fsS "$api/healthz" >/dev/null || fail "the backend does not answer $api/healthz: just run backend first"

column() {
  awk -F '\t' -v actor="$1" -v want="$2" '
    $1 == "actor" { for (i = 1; i <= NF; i++) if ($i == want) col = i; next }
    $1 == actor && col { print $col; exit }
  ' "$accounts"
}

sql() {
  apps/mobile/qa/journeys/psql.sh -v ON_ERROR_STOP=1 -tA "$@"
}

uuid7() {
  echo "encode(set_bit(set_bit(overlay(uuid_send(gen_random_uuid()) placing
    substring(int8send(floor(extract(epoch FROM ${1:-clock_timestamp()}) * 1000)::bigint) FROM 3) FROM 1 FOR 6),
    52, 1), 53, 1), 'hex')::uuid"
}

user_id() {
  local did id
  did="$(column "$1" privy_user_id)"
  id="$(sql -v did="$did" <<<"SELECT id FROM users WHERE privy_user_id = :'did'")"
  [[ -n "$id" ]] || fail "actor $1 ($did) has no users row: sign in as $1 once first"
  echo "$id"
}

token() {
  scripts/qa/monacoctl.sh dev token --user "$1" "${@:2}" --ttl 1h || fail "monacoctl dev token --user $1 failed"
}

call() {
  local method="$1" path="$2" bearer="$3" body="${4:-}"
  local args=(-sS -X "$method" "$api$path" -H "Authorization: Bearer $bearer" -H "Content-Type: application/json"
    -w $'\n%{http_code}')
  if [[ "$method" != GET ]]; then
    args+=(-H "Idempotency-Key: $(uuidgen | tr '[:upper:]' '[:lower:]')")
  fi
  if [[ -n "$body" ]]; then
    args+=(-d "$body")
  fi
  local response status
  response="$(curl "${args[@]}")"
  status="${response##*$'\n'}"
  [[ "$status" == 2* ]] || fail "$method $path answered $status: ${response%$'\n'*}"
  printf '%s' "${response%$'\n'*}"
}

field() {
  python3 -c '
import json, sys
value = json.load(sys.stdin)
for key in sys.argv[1].split("."):
    value = value.get(key) if isinstance(value, dict) else None
print("" if value is None else value)
' "$1"
}

admit() {
  local joiner="$1" creator="$2" cabal="$3" request
  request="$(call POST "/v1/cabals/$cabal/access-requests" "$joiner" 2>/dev/null | field id)" || return 0
  [[ -n "$request" ]] || return 0
  call POST "/v1/cabals/$cabal/access-requests/$request/decision" "$creator" '{"decision":"approve"}' >/dev/null
}

ready_actor() {
  local actor="$1" bearer="$2" name
  sql -v did="$(column "$actor" privy_user_id)" >/dev/null \
    <<<"UPDATE users SET auth_state = 'ONBOARDING_COMPLETED', auth_state_changed_at = now() WHERE privy_user_id = :'did' AND auth_state <> 'ONBOARDING_COMPLETED'"
  name="$(column "$actor" name)"
  if [[ "$(call GET /v1/me "$bearer" | field display_name)" != "$name" ]]; then
    call PATCH /v1/me "$bearer" "{\"display_name\":\"$name\"}" >/dev/null
  fi
}

expire_open_proposals() {
  sql -v a="$1" -v b="$2" <<SQL
WITH due AS (
  UPDATE proposals SET status = 'expired', updated_at = now()
  WHERE status = 'open'
    AND id IN (SELECT proposal_id FROM proposal_voters WHERE voter_id IN (:'a', :'b'))
  RETURNING id, cabal_id
), appended AS (
  INSERT INTO events (id, aggregate_type, aggregate_id, type, payload, actor_type, actor_id, created_at)
  SELECT $(uuid7), 'proposal', id, 'proposal.expired',
    jsonb_build_object('v', 1, 'proposal_id', id, 'cabal_id', cabal_id),
    'system', 'poller.governance.proposal_expiry', now()
  FROM due
  RETURNING 1
)
SELECT count(*) FROM appended
SQL
}

open_proposal() {
  local cabal="$1" proposer="$2" thesis="$3" status="$4" created="$5" expires="$6"
  sql -v cabal="$cabal" -v proposer="$proposer" -v thesis="$thesis" -v status="$status" \
    -v created="$created" -v expires="$expires" <<SQL
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
    quote_out, :'status', (SELECT threshold FROM cabals WHERE id = :'cabal'), now() + (:'expires')::interval,
    now() + (:'created')::interval,
    CASE WHEN :'status' = 'open' THEN now() + (:'created')::interval ELSE now() + (:'expires')::interval END
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
), closed AS (
  INSERT INTO events (id, aggregate_type, aggregate_id, type, payload, actor_type, actor_id, created_at)
  SELECT $(uuid7 "clock_timestamp() + interval '1 millisecond'"), 'proposal', id, 'proposal.' || :'status',
    jsonb_build_object('v', 1, 'proposal_id', id, 'cabal_id', cabal_id),
    'system', 'poller.governance.proposal_expiry', expires_at
  FROM proposal WHERE status <> 'open'
)
SELECT id FROM proposal
SQL
}

hand_off() {
  python3 - "$handoff" "$@" <<'PY'
import json, os, sys
path, pairs = sys.argv[1], sys.argv[2:]
values = json.load(open(path)) if os.path.exists(path) and os.path.getsize(path) else {}
values.update(zip(pairs[::2], pairs[1::2]))
json.dump(values, open(path, "w"))
PY
}

retry() {
  local tries="$1" delay="$2" out
  shift 2
  for ((i = 0; i < tries; i++)); do
    if out="$("$@" 2>/dev/null)"; then
      printf '%s' "$out"
      return 0
    fi
    sleep "$delay"
  done
  return 1
}

id_a="$(user_id A)"
token_a="$(token "$id_a")"
ready_actor A "$token_a"

expire_open_proposals "$id_a" "$id_a" >/dev/null

host_token="$(token new --pool catchup-host)"
call PATCH /v1/me "$host_token" '{"display_name":"QA catchup host"}' >/dev/null
host_id="$(call GET /v1/me "$host_token" | field id)"
cabal_name="QA catch up $run"
cabal_id="$(call POST /v1/cabals "$host_token" \
  "{\"name\":\"$cabal_name\",\"join_mode\":\"request\",\"voter_mode\":\"all\",\"threshold\":\"majority\",\"proposal_expiry_seconds\":86400}" |
  field id)"
admit "$token_a" "$host_token" "$cabal_id"

waiting_id="$(open_proposal "$cabal_id" "$host_id" "QA catch up waiting $run" open '0 seconds' '86400 seconds')"
result_id="$(open_proposal "$cabal_id" "$host_id" "QA catch up result $run" open '1 second' '86400 seconds')"
[[ -n "$waiting_id" && -n "$result_id" ]] || fail "no TSLAx row in assets, so no proposal was written"

call POST "/v1/proposals/$result_id/votes" "$token_a" '{"choice":"yes"}' >/dev/null
call POST "/v1/proposals/$result_id/votes" "$host_token" '{"choice":"yes"}' >/dev/null

comment_a="$(retry 20 2 call POST "/v1/proposals/$waiting_id/comments" "$token_a" '{"body":"QA catch up question"}' | field id)"
[[ -n "$comment_a" ]] || fail "A's comment on $waiting_id was not stored: the proposal has no feed item yet"
reply="$(call POST "/v1/proposals/$waiting_id/comments" "$host_token" \
  "{\"body\":\"QA catch up answer\",\"parent_comment_id\":\"$comment_a\"}" | field id)"

hand_off cabalName "$cabal_name" cabalID "$cabal_id" waitingKey "proposal:$waiting_id" replyKey "comment:$reply" \
  resultKey "proposal:$result_id"

for ((i = 0; i < 30; i++)); do
  read -r waiting replies results < <(call GET /v1/me/catch-up "$token_a" | python3 -c '
import json, sys
body = json.load(sys.stdin)
print(*(sum(1 for item in body[s] if item["key"] == k) for s, k in
  (("waiting", sys.argv[1]), ("replies", sys.argv[2]), ("results", sys.argv[3]))))' \
    "proposal:$waiting_id" "comment:$reply" "proposal:$result_id")
  [[ "$waiting $replies $results" == "1 1 1" ]] && break
  sleep 2
done
[[ "$waiting $replies $results" == "1 1 1" ]] ||
  fail "GET /v1/me/catch-up for A lists waiting=$waiting replies=$replies results=$results of the three seeded items"
echo "seeded S1: cabal '$cabal_name' ($cabal_id), waiting $waiting_id, reply comment $reply, result $result_id"
