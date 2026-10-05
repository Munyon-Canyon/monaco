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
#   qa_seed_scenario <name> [actor ...]   bin/monacoctl dev seed-scenario, each actor owning its scenario letter;
#                                         exports each seeded id uppercased (CABAL_ID, ASSET_MINT, ...)
#   qa_fake_usdc <actor> <usdc>           sets the actor's on-chain USDC on bin/fakes (QA_FAKES_URL), e.g. 25 or 0.5
#
# QA_FAKE_RPC=1 exported before sourcing points SOLANA_RPC_URL at bin/fakes, so a backend started from this
# shell (just run backend, journey.py) reads balances that qa_fake_usdc sets instead of mainnet.

QA_ROOT="$(git rev-parse --show-toplevel)"
QA_API="${MONACO_API_BASE_URL:-http://127.0.0.1:8080}"
QA_ACCOUNTS="$QA_ROOT/apps/mobile/qa/journeys/accounts.tsv"
QA_FAKES="${QA_FAKES_URL:-http://127.0.0.1:8099}"
if [[ "${QA_FAKE_RPC:-}" == 1 ]]; then
  export SOLANA_RPC_URL="$QA_FAKES/rpc/"
fi

# with-dotenv-local.sh prints what it injected; a setup log only needs the command's own errors.
_qa_quiet() {
  grep -v -e '^with-dotenv-local:' -e 'injected env' >&2 || true
}

_qa_monacoctl() {
  if [[ ! -x "$QA_ROOT/bin/monacoctl" ]]; then
    echo "bin/monacoctl is missing: run just build backend first" >&2
    return 1
  fi
  # monacoctl refuses unknown MONACO_ variables: a run's account overrides are MONACO_QA_, and
  # MONACO_API_BASE_URL is the journey runner's, which this file reads as QA_API.
  local name unset=(-u MONACO_API_BASE_URL)
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
  # zsh ties path to PATH and makes status read-only, so neither names a local here.
  local actor="$1" method="$2" route="$3" body="${4:-}" token code out
  token="$(qa_token "$actor")" || return 1
  out="$(mktemp)"
  local args=(-sS -o "$out" -w '%{http_code}' -X "$method" "$QA_API$route"
    -H "Authorization: Bearer $token" -H "Content-Type: application/json")
  if [[ "$method" != GET ]]; then
    args+=(-H "Idempotency-Key: $(uuidgen | tr '[:upper:]' '[:lower:]')")
  fi
  if [[ -n "$body" ]]; then
    args+=(-d "$body")
  fi
  if ! code="$(curl "${args[@]}")"; then
    rm -f "$out"
    echo "qa_api $method $route: no answer from $QA_API" >&2
    return 1
  fi
  if [[ "$code" != 2?? ]]; then
    echo "qa_api $actor $method $route: HTTP $code" >&2
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

qa_seed_scenario() {
  local name="$1" actor id key value out args=()
  shift
  for actor in "$@"; do
    id="$(qa_user_id "$actor")" || return 1
    args+=(--actor "$actor=$id")
  done
  out="$(mktemp)"
  if ! _qa_monacoctl dev seed-scenario "$name" ${args[@]+"${args[@]}"} > "$out"; then
    rm -f "$out"
    return 1
  fi
  while read -r key value; do
    export "$(tr '[:lower:]' '[:upper:]' <<< "$key")=$value"
  done < "$out"
  rm -f "$out"
}

qa_fake_usdc() {
  local actor="$1" usdc="$2" id address micros body
  id="$(qa_user_id "$actor")" || return 1
  address="$(qa_sql -v id="$id" <<< "SELECT address FROM user_wallets WHERE user_id = :'id'")"
  if [[ -z "$address" ]]; then
    echo "actor $actor has no member wallet: sign in as $actor once first" >&2
    return 1
  fi
  if ! micros="$(python3 -c 'import decimal,sys; v = decimal.Decimal(sys.argv[1]) * 10**6
assert v >= 0 and v == v.to_integral_value(), "usdc must be >= 0 with at most 6 decimals"
print(int(v))' "$usdc")"; then
    echo "qa_fake_usdc: bad amount $usdc" >&2
    return 1
  fi
  body="$(printf '{"owner":"%s","mint":"%s","amount":"%s","decimals":6}' \
    "$address" "${SOLANA_USDC_MINT:-EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v}" "$micros")"
  if ! curl -fsS -X POST "$QA_FAKES/_balance" -H "Content-Type: application/json" -d "$body"; then
    echo "qa_fake_usdc: POST $QA_FAKES/_balance failed: is bin/fakes running there?" >&2
    return 1
  fi
}
