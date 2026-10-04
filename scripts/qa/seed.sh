# shellcheck shell=bash
# Seed helpers for journey setup scripts. Source it, do not run it:
#
#   source scripts/qa/seed.sh
#   qa_api_ready
#   cabal="$(qa_api A POST /v1/cabals '{"name":"QA pot"}' | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')"
#
# Seed through the API first, flow seeds second, and SQL only for a state no route can reach
# (docs/journeys/README.md). A test never taps to create its starting state.
#
#   qa_api_ready                          fails with a hint when the backend does not answer /healthz
#   qa_user_id <actor>                    the users.id of an actor in accounts.tsv
#   qa_token <actor>                      a 1 h bearer token for the actor
#   qa_api <actor> <METHOD> <path> [json] calls the API as the actor; prints the body, fails on non-2xx
#   qa_sql [psql args]                    psql on the .env.local database, query on stdin
#   qa_flow_seed <flow> <outcome>         bin/monacoctl flows seed

QA_ROOT="$(git rev-parse --show-toplevel)"
QA_API="${MONACO_API_BASE_URL:-http://127.0.0.1:8080}"
QA_ACCOUNTS="$QA_ROOT/apps/mobile/qa/journeys/accounts.tsv"

# with-dotenv-local.sh prints what it injected; a setup log only needs the command's own errors.
_qa_quiet() {
  grep -v -e '^with-dotenv-local:' -e 'injected env' >&2 || true
}

_qa_monacoctl() {
  if [[ ! -x "$QA_ROOT/bin/monacoctl" ]]; then
    echo "bin/monacoctl is missing: run just build backend first" >&2
    return 1
  fi
  # The backend refuses unknown MONACO_ variables, and a run's account overrides are MONACO_QA_.
  local name unset=()
  while read -r name; do
    unset+=(-u "$name")
  done < <(compgen -e | grep '^MONACO_QA_' || true)
  (cd "$QA_ROOT" && env ${unset[@]+"${unset[@]}"} scripts/with-dotenv-local.sh bin/monacoctl "$@" 2> >(_qa_quiet))
}

qa_api_ready() {
  if ! curl -fs --max-time 5 "$QA_API/healthz" > /dev/null; then
    echo "the backend does not answer $QA_API/healthz: start it with just run backend" >&2
    return 1
  fi
}

qa_sql() {
  (cd "$QA_ROOT" && scripts/with-dotenv-local.sh apps/mobile/qa/journeys/psql.sh -v ON_ERROR_STOP=1 -tA "$@" \
    2> >(_qa_quiet))
}

_qa_account() {
  awk -F '\t' -v actor="$1" -v want="$2" \
    '$1 == "actor" { for (i = 1; i <= NF; i++) if ($i == want) col = i; next } $1 == actor && col { print $col; exit }' \
    "$QA_ACCOUNTS"
}

qa_user_id() {
  local did id
  did="$(_qa_account "$1" privy_user_id)"
  if [[ -z "$did" ]]; then
    echo "actor $1 has no privy_user_id in $QA_ACCOUNTS" >&2
    return 1
  fi
  id="$(qa_sql -v did="$did" <<< "SELECT id FROM users WHERE privy_user_id = :'did'")"
  if [[ -z "$id" ]]; then
    echo "actor $1 ($did) has no users row: sign in as $1 once first" >&2
    return 1
  fi
  echo "$id"
}

qa_token() {
  local id
  id="$(qa_user_id "$1")" || return 1
  _qa_monacoctl dev token --user "$id" --ttl 1h
}

qa_api() {
  local actor="$1" method="$2" path="$3" body="${4:-}" token status out
  token="$(qa_token "$actor")" || return 1
  out="$(mktemp)"
  local args=(-sS -o "$out" -w '%{http_code}' -X "$method" "$QA_API$path"
    -H "Authorization: Bearer $token" -H "Content-Type: application/json")
  if [[ "$method" != GET ]]; then
    args+=(-H "Idempotency-Key: $(uuidgen | tr '[:upper:]' '[:lower:]')")
  fi
  if [[ -n "$body" ]]; then
    args+=(-d "$body")
  fi
  if ! status="$(curl "${args[@]}")"; then
    rm -f "$out"
    echo "qa_api $method $path: no answer from $QA_API" >&2
    return 1
  fi
  if [[ "$status" != 2?? ]]; then
    echo "qa_api $actor $method $path: HTTP $status" >&2
    cat "$out" >&2
    echo >&2
    rm -f "$out"
    return 1
  fi
  cat "$out"
  rm -f "$out"
}

qa_flow_seed() {
  _qa_monacoctl flows seed "$1" "$2"
}
