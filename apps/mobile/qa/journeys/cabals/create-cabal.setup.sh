#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
scenario="${1:?journey.py passes the scenario id}"
while read -r name; do
  unset "$name"
done < <(compgen -e | grep '^MONACO_QA_')
api="${MONACO_API_BASE_URL:-http://127.0.0.1:8080}"
unset MONACO_API_BASE_URL
accounts="apps/mobile/qa/journeys/accounts.tsv"

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

sql() {
  scripts/with-dotenv-local.sh apps/mobile/qa/journeys/psql.sh -v ON_ERROR_STOP=1 -tA "$@" \
    2> >(grep -v -e '^with-dotenv-local:' -e 'injected env' >&2)
}

user_id() {
  local did id
  did="$(column "$1" privy_user_id)"
  id="$(sql -v did="$did" <<<"SELECT id FROM users WHERE privy_user_id = :'did'")"
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
  local actor="$1" bearer="$2"
  sql -v did="$(column "$actor" privy_user_id)" >/dev/null \
    <<<"UPDATE users SET auth_state = 'ONBOARDING_COMPLETED', auth_state_changed_at = now() WHERE privy_user_id = :'did' AND auth_state <> 'ONBOARDING_COMPLETED'"
  local name
  name="$(column "$actor" name)"
  if [[ "$(call GET /v1/me "$bearer" | field display_name)" != "$name" ]]; then
    call PATCH /v1/me "$bearer" "{\"display_name\":\"$name\"}" >/dev/null
  fi
}

if [[ "$scenario" != S4 ]]; then
  echo "seeded: nothing for $scenario"
  exit 0
fi

ready_actor A "$(token "$(user_id A)")"
ready_actor B "$(token "$(user_id B)")"
echo "seeded: A and B are past onboarding and named $(column A name) and $(column B name)"
