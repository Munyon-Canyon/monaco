#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
while read -r name; do
  unset "$name"
done < <(compgen -e | grep '^MONACO_QA_')
api="${MONACO_API_BASE_URL:-http://127.0.0.1:8080}"
accounts="${QA_ACCOUNTS_FILE:-apps/mobile/qa/journeys/accounts.tsv}"

if [[ ! -x bin/monacoctl ]]; then
  echo "run just build backend first" >&2
  exit 1
fi
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
  if ! out="$(scripts/with-dotenv-local.sh bin/monacoctl dev token --user "$1" --ttl 1h 2>"$error_file")"; then
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

host_token="$(token new)"
call PATCH /v1/me "$host_token" '{"display_name":"QA host"}' >/dev/null
name="QA pot $(date +%H%M%S)"
cabal="$(call POST /v1/cabals "$host_token" \
  "{\"name\":\"$name\",\"join_mode\":\"request\",\"voter_mode\":\"all\",\"threshold\":\"majority\",\"proposal_expiry_seconds\":86400}" |
  field id)"
call POST "/v1/cabals/$cabal/invites" "$host_token" "{\"handle\":\"$handle_a\"}" >/dev/null

echo "seeded: $name, open, invites @$handle_a; @$handle_b has no pending invites"
