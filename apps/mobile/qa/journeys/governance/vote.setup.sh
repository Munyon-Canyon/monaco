#!/usr/bin/env bash
# journey.py runs this before each scenario of docs/journeys/governance/vote.md with the scenario id.
# Every scenario gets its own cabal from a new "QA host" user, with A and B as members and one open
# buy proposal by the host. S3 adds an expired proposal. The proposal rows are written the way
# governance.ProposeTrade writes them, because the propose route needs a Jupiter route and pot funds.
set -euo pipefail

scenario="${1:?usage: vote.setup.sh <S1|S2|S3>}"
case "$scenario" in
  S1 | S2 | S3) ;;
  *) echo "vote.setup.sh: unknown scenario '$scenario'" >&2 && exit 1 ;;
esac

cd "$(git rev-parse --show-toplevel)"
if [[ "${VOTE_SETUP_ENV:-}" != 1 ]]; then
  VOTE_SETUP_ENV=1 exec scripts/with-dotenv-local.sh "$0" "$@" 2> >(grep -v -e '^with-dotenv-local:' -e 'injected env' >&2)
fi

run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
handoff="${MONACO_QA_HANDOFF:?journey.py sets MONACO_QA_HANDOFF}"
while read -r name; do
  unset "$name"
done < <(compgen -e | grep '^MONACO_QA_')
api="${MONACO_API_BASE_URL:-http://127.0.0.1:8080}"
unset MONACO_API_BASE_URL
accounts="${QA_ACCOUNTS_FILE:-apps/mobile/qa/journeys/accounts.tsv}"

fail() {
  echo "vote.setup.sh $scenario: $*" >&2
  exit 1
}

[[ -x bin/monacoctl ]] || fail "run just build backend first"
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
  bin/monacoctl dev token --user "$1" --ttl 1h || fail "monacoctl dev token --user $1 failed"
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
    quote_out_amount, status, expires_at, created_at, updated_at)
  SELECT $(uuid7), :'cabal', :'proposer', 'buy', symbol, mint, 5000000, NULL, :'thesis',
    quote_out, :'status', now() + (:'expires')::interval, now() + (:'created')::interval,
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
values.pop("closedProposalID", None)
values.update(zip(pairs[::2], pairs[1::2]))
json.dump(values, open(path, "w"))
PY
}

id_a="$(user_id A)"
id_b="$(user_id B)"
token_a="$(token "$id_a")"
token_b="$(token "$id_b")"
ready_actor A "$token_a"
ready_actor B "$token_b"

closed="$(expire_open_proposals "$id_a" "$id_b")"

# Reuse the newest QA host: Privy caps the app's users, so a new one per scenario eventually fails.
host_user="$(sql <<<"SELECT id FROM users WHERE display_name = 'QA host' ORDER BY id DESC LIMIT 1")"
if [[ -n "$host_user" ]]; then
  host_token="$(token "$host_user")"
else
  host_token="$(token new)"
  call PATCH /v1/me "$host_token" '{"display_name":"QA host"}' >/dev/null
fi
host_id="$(call GET /v1/me "$host_token" | field id)"
cabal_name="QA vote $run $scenario"
cabal_id="$(call POST /v1/cabals "$host_token" \
  "{\"name\":\"$cabal_name\",\"join_mode\":\"request\",\"voter_mode\":\"all\",\"threshold\":\"majority\",\"proposal_expiry_seconds\":86400}" |
  field id)"
admit "$token_a" "$host_token" "$cabal_id"
admit "$token_b" "$host_token" "$cabal_id"

proposal_id="$(open_proposal "$cabal_id" "$host_id" "QA vote $run" open '0 seconds' '86400 seconds')"
[[ -n "$proposal_id" ]] || fail "no TSLAx row in assets, so no proposal was written"
pairs=(cabalName "$cabal_name" cabalID "$cabal_id" proposalID "$proposal_id" "${scenario}ProposalID" "$proposal_id")

if [[ "$scenario" == S3 ]]; then
  closed_id="$(open_proposal "$cabal_id" "$host_id" "QA closed $run" expired '-2 days' '-1 day')"
  pairs+=(closedProposalID "$closed_id")
fi
hand_off "${pairs[@]}"

pending="$(call GET /v1/me/pending-votes "$token_a" |
  python3 -c 'import json, sys; print(" ".join(v["proposal_id"] for v in json.load(sys.stdin)))')"
[[ " $pending " == *" $proposal_id "* ]] || fail "GET /v1/me/pending-votes for A does not list $proposal_id"

read -r voters needed yes < <(call GET "/v1/proposals/$proposal_id" "$token_a" |
  python3 -c 'import json, sys; t = json.load(sys.stdin)["tally"]; print(t["voters"], t["needed"], t["yes"])')
echo "seeded $scenario: expired $closed open proposals of A or B, cabal '$cabal_name' ($cabal_id), proposal $proposal_id${closed_id:+, closed proposal $closed_id}"
echo "tally for A: $yes yes of $voters voters, $needed needed"
