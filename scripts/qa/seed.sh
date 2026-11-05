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
#                                         a retryable 429 is retried for up to 30 s, honoring Retry-After
#   qa_sql [psql args]                    psql on the .env.local database, query on stdin
#   qa_flow_seed <flow> <outcome>         bin/monacoctl flows seed
#   qa_seed_scenario <name> [actor ...]   bin/monacoctl dev seed-scenario, each actor owning its scenario letter;
#                                         exports each seeded id uppercased (CABAL_ID, ASSET_MINT, ...)
#   qa_add_photo <udid>                   adds apps/mobile/qa/journeys/fixtures/photo.jpg to the simulator's Photos
#   qa_fake_treasury_positions <cabal id> sets the cabal treasury's on-chain tokens on bin/fakes to its cabal_positions rows
#   qa_fake_trade_wallet <cabal id>       registers the cabal treasury's Privy wallet id and address on bin/fakes (rpc: fake-trade)
#   qa_fake_member_wallet <actor>         re-registers the actor's stored member wallet id and address on bin/fakes (they forget wallets on restart)
#   qa_fake_swap <cabal id> <status>      sets the next swap outcome (success, failed or pending) for that treasury on bin/fakes
#   qa_fake_usdc <actor> <usdc>           sets the actor's on-chain USDC on bin/fakes (QA_FAKES_URL), e.g. 25 or 0.5
#   qa_fake_holding <actor> <symbol> <usdc> <cabal name>
#                                         rpc: fake-trade: creates a cabal, funds it from the actor's fake USDC, then proposes
#                                         and votes a buy of half of <usdc> of <symbol> through the API (it buys half, leaving cash
#                                         for the fee headroom the proposal check reserves); prints the cabal id when the position is confirmed
#
# QA_FAKE_RPC=1 exported before sourcing points SOLANA_RPC_URL at bin/fakes, so a backend started from this
# shell (just run backend, journey.py) reads balances that qa_fake_usdc sets instead of mainnet.

QA_ROOT="$(git rev-parse --show-toplevel)"
QA_API="${MONACO_API_BASE_URL:-http://127.0.0.1:8080}"
# journey.py points QA_ACCOUNTS_FILE at the run's accounts: each actor's row is the login the run holds for it.
QA_ACCOUNTS="${QA_ACCOUNTS_FILE:-$QA_ROOT/apps/mobile/qa/journeys/accounts.tsv}"
QA_FAKES="${QA_FAKES_URL:-http://127.0.0.1:8099}"
if [[ "${QA_FAKE_RPC:-}" == 1 ]]; then
  export SOLANA_RPC_URL="$QA_FAKES/rpc/"
fi

# with-dotenv-local.sh prints what it injected; a setup log only needs the command's own errors.
_qa_quiet() {
  grep -v -e '^with-dotenv-local:' -e 'injected env' >&2 || true
}

_qa_monacoctl() {
  "$QA_ROOT/scripts/qa/monacoctl.sh" "$@" 2> >(_qa_quiet)
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

_qa_retryable() {
  python3 -c 'import json,sys; sys.exit(0 if json.load(open(sys.argv[1])).get("retryable") is True else 1)' "$1" \
    2> /dev/null
}

qa_api() {
  # zsh ties path to PATH and makes status read-only, so neither names a local here.
  local actor="$1" method="$2" route="$3" body="${4:-}" token code out headers delay after
  local tries=0 waited=0 backoff=2
  token="$(qa_token "$actor")" || return 1
  out="$(mktemp)"
  headers="$(mktemp)"
  local args=(-sS -o "$out" -D "$headers" -w '%{http_code}' -X "$method" "$QA_API$route"
    -H "Authorization: Bearer $token" -H "Content-Type: application/json")
  if [[ "$method" != GET ]]; then
    args+=(-H "Idempotency-Key: $(uuidgen | tr '[:upper:]' '[:lower:]')")
  fi
  if [[ -n "$body" ]]; then
    args+=(-d "$body")
  fi
  while :; do
    if ! code="$(curl "${args[@]}")"; then
      rm -f "$out" "$headers"
      echo "qa_api $method $route: no answer from $QA_API" >&2
      return 1
    fi
    if [[ "$code" != 429 ]] || ! _qa_retryable "$out"; then
      break
    fi
    after="$(awk 'tolower($1) == "retry-after:" { sub(/\r$/, "", $2); print $2 }' "$headers")"
    if [[ "$after" =~ ^[0-9]+$ ]]; then
      delay="$after"
    else
      delay="$backoff"
      backoff=$((backoff < 16 ? backoff * 2 : 16))
    fi
    if ((waited + delay > 30)); then
      break
    fi
    tries=$((tries + 1))
    echo "qa_api: 429 rate_limited on $method $route, retry $tries in ${delay}s" >&2
    sleep "$delay"
    waited=$((waited + delay))
  done
  rm -f "$headers"
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

# qa_admit <joiner> <creator> <cabal>: every cabal is request-to-join, so the joiner asks
# and the creator approves. A joiner who is already in, or already asked, is left alone.
qa_admit() {
  local joiner="$1" creator="$2" cabal="$3" request
  request="$(qa_api "$joiner" POST "/v1/cabals/$cabal/access-requests" 2>/dev/null |
    python3 -c 'import json,sys; print(json.load(sys.stdin).get("id", ""))' 2>/dev/null)" || return 0
  [[ -n "$request" ]] || return 0
  qa_api "$creator" POST "/v1/cabals/$cabal/access-requests/$request/decision" '{"decision":"approve"}' >/dev/null
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

qa_fake_treasury_positions() {
  local cabal="${1:?usage: qa_fake_treasury_positions <cabal id>}" address rows asset units decimals body
  address="$(qa_sql -v cabal="$cabal" <<< "SELECT address FROM treasury_wallets WHERE cabal_id = :'cabal'")"
  if [[ -z "$address" ]]; then
    echo "qa_fake_treasury_positions: cabal $cabal has no treasury wallet" >&2
    return 1
  fi
  rows="$(qa_sql -v cabal="$cabal" <<< "SELECT p.asset || '|' || p.units::text || '|' || coalesce(a.decimals, 6)
    FROM cabal_positions p LEFT JOIN assets a ON a.mint = p.asset WHERE p.cabal_id = :'cabal' AND p.units > 0")"
  while IFS='|' read -r asset units decimals; do
    [[ -n "$asset" ]] || continue
    body="$(printf '{"owner":"%s","mint":"%s","amount":"%s","decimals":%s}' "$address" "$asset" "$units" "$decimals")"
    if ! curl -fsS -X POST "$QA_FAKES/_balance" -H "Content-Type: application/json" -d "$body"; then
      echo "qa_fake_treasury_positions: POST $QA_FAKES/_balance failed: is bin/fakes running there?" >&2
      return 1
    fi
  done <<< "$rows"
}

_qa_treasury() {
  local cabal="$1" row
  row="$(qa_sql -v cabal="$cabal" <<< "SELECT privy_wallet_id, address FROM treasury_wallets WHERE cabal_id = :'cabal'")"
  if [[ -z "$row" ]]; then
    echo "cabal $cabal has no treasury wallet" >&2
    return 1
  fi
  printf '%s\n' "$row"
}

_qa_register_wallet() {
  local caller="$1" id="$2" address="$3" body
  body="$(printf '{"id":"%s","address":"%s"}' "$id" "$address")"
  if ! curl -fsS -X POST "$QA_FAKES/_wallet" -H "Content-Type: application/json" -d "$body"; then
    echo "$caller: POST $QA_FAKES/_wallet failed: is bin/fakes running there?" >&2
    return 1
  fi
}

qa_fake_trade_wallet() {
  local cabal="${1:?usage: qa_fake_trade_wallet <cabal id>}" row
  row="$(_qa_treasury "$cabal")" || return 1
  _qa_register_wallet qa_fake_trade_wallet "${row%%|*}" "${row#*|}"
}

qa_fake_member_wallet() {
  local actor="${1:?usage: qa_fake_member_wallet <actor>}" id row
  id="$(qa_user_id "$actor")" || return 1
  row="$(qa_sql -v id="$id" <<< "SELECT privy_wallet_id, address FROM user_wallets WHERE user_id = :'id'")"
  if [[ -z "$row" ]]; then
    echo "actor $actor has no member wallet: sign in as $actor once first" >&2
    return 1
  fi
  _qa_register_wallet qa_fake_member_wallet "${row%%|*}" "${row#*|}"
}

qa_fake_swap() {
  local cabal="${1:?usage: qa_fake_swap <cabal id> success|failed|pending}" outcome="${2:-}" row body
  case "$outcome" in
    success) outcome=Success ;;
    failed) outcome=Failed ;;
    pending) outcome=Pending ;;
    *) echo "usage: qa_fake_swap <cabal id> success|failed|pending" >&2; return 1 ;;
  esac
  row="$(_qa_treasury "$cabal")" || return 1
  body="$(printf '{"owner":"%s","status":"%s"}' "${row#*|}" "$outcome")"
  if ! curl -fsS -X POST "$QA_FAKES/_swap" -H "Content-Type: application/json" -d "$body"; then
    echo "qa_fake_swap: POST $QA_FAKES/_swap failed: is bin/fakes running there?" >&2
    return 1
  fi
}

_qa_json() {
  python3 -c 'import json,sys; print(json.load(sys.stdin)[sys.argv[1]])' "$1"
}

# qa_fake_holding gets a holding the way a member does, never by writing money tables. A buy proposal checks the
# ledger's cash and the treasury's on-chain USDC (the lower of the two), so the cabal is funded with the fund sweep first.
# The fakes land the sweep but do not apply transfers, so the setup mirrors the ledger onto the fake chain before it proposes. It buys half, leaving cash for the
# fee headroom the proposal check reserves (cashCovers needs the trade plus the mint's transfer-fee headroom).
# Prints the cabal id only.
qa_fake_holding() {
  local actor="${1:?usage: qa_fake_holding <actor> <symbol> <usdc> <cabal name>}" symbol="${2:?symbol}" usdc="${3:?usdc}"
  local name="${4:?cabal name}" cabal micros transfer state proposal mint units n
  if ! micros="$(python3 -c 'import decimal,sys; v = decimal.Decimal(sys.argv[1]) * 10**6
assert v >= 1000000 and v == v.to_integral_value(), "usdc must be at least 1 with at most 6 decimals"
print(int(v))' "$usdc")"; then
    echo "qa_fake_holding: bad amount $usdc" >&2
    return 1
  fi
  mint="$(qa_sql -v sym="$symbol" <<< "SELECT mint FROM assets WHERE symbol = :'sym'")"
  if [[ -z "$mint" ]]; then
    echo "qa_fake_holding: no asset $symbol in the catalogue" >&2
    return 1
  fi
  cabal="$(qa_api "$actor" POST /v1/cabals "$(printf '{"name":"%s","join_mode":"request","voter_mode":"all","threshold":"majority","proposal_expiry_seconds":86400}' "$name")" |
    _qa_json id)" || return 1
  qa_fake_trade_wallet "$cabal" >&2 || return 1
  qa_fake_member_wallet "$actor" >&2 || return 1
  qa_fake_usdc "$actor" "$usdc" >&2 || return 1
  qa_fake_swap "$cabal" success >&2 || return 1
  transfer="$(qa_api "$actor" POST "/v1/cabals/$cabal/fund" "$(printf '{"amount_micros":"%s"}' "$micros")" |
    _qa_json transfer_id)" || return 1
  for ((n = 0; n < 30; n++)); do
    state="$(qa_api "$actor" GET "/v1/fund-transfers/$transfer" | _qa_json status)" || return 1
    case "$state" in
      settled) break ;;
      failed)
        echo "qa_fake_holding: fund transfer $transfer failed" >&2
        return 1
        ;;
    esac
    sleep 2
  done
  if [[ "$state" != settled ]]; then
    echo "qa_fake_holding: fund transfer $transfer is still '$state' after 60 s" >&2
    return 1
  fi
  qa_fake_treasury_positions "$cabal" >&2 || return 1
  proposal="$(qa_api "$actor" POST "/v1/cabals/$cabal/proposals" \
    "$(printf '{"kind":"buy","symbol":"%s","usdc_micros":%s}' "$symbol" "$((micros / 2))")" | _qa_json id)" || return 1
  qa_api "$actor" POST "/v1/proposals/$proposal/votes" '{"choice":"yes"}' >/dev/null || return 1
  for ((n = 0; n < 30; n++)); do
    units="$(qa_sql -v cabal="$cabal" -v mint="$mint" \
      <<< "SELECT units FROM cabal_positions WHERE cabal_id = :'cabal' AND asset = :'mint' AND units > 0")"
    [[ -z "$units" ]] || break
    sleep 2
  done
  if [[ -z "$units" ]]; then
    echo "qa_fake_holding: no $symbol position in cabal $cabal after 60 s: the buy did not execute" >&2
    return 1
  fi
  qa_fake_treasury_positions "$cabal" >&2 || return 1
  echo "$cabal"
}

qa_add_photo() {
  xcrun simctl addmedia "${1:?usage: qa_add_photo <udid>}" "$QA_ROOT/apps/mobile/qa/journeys/fixtures/photo.jpg"
}
