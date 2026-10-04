#!/usr/bin/env bash
set -euo pipefail

api="${MONACO_QA_API_BASE_URL:-http://127.0.0.1:8080}"
accounts="apps/mobile/qa/journeys/accounts.tsv"
privy_user_id="$(awk -F '\t' '
  $1 == "actor" {
    for (i = 1; i <= NF; i++) {
      if ($i == "actor") actor_column = i
      if ($i == "privy_user_id") privy_user_id_column = i
    }
    next
  }
  actor_column != "" && $actor_column == "A" { print $privy_user_id_column; exit }
' "$accounts")"

if [[ -z "$privy_user_id" ]]; then
  echo "missing Privy user ID for actor A" >&2
  exit 1
fi

query="SELECT id FROM users WHERE privy_user_id = :'value' LIMIT 1"
set +e
user_id="$(printf '%s\n' "$query" | scripts/with-dotenv-local.sh apps/mobile/qa/journeys/psql.sh \
  -v ON_ERROR_STOP=1 -v value="$privy_user_id" -tA 2>/dev/null)"
status=$?
set -e
if [[ $status -ne 0 ]]; then
  echo "database cannot be reached (psql exit $status)" >&2
  exit 2
fi
if [[ -z "$user_id" ]]; then
  echo "no users row for actor A"
  exit 1
fi

token_file="$(mktemp)"
trap 'rm -f "$token_file"' EXIT
for name in $(compgen -e | grep '^MONACO_QA_'); do unset "$name"; done
# shellcheck disable=SC2016 # $1 expands in the bash -c child, not here.
mint='cd apps/backend && exec go run ./cmd/monacoctl dev token --user "$1"'
if ! scripts/with-dotenv-local.sh bash -c "$mint" bash "$user_id" >"$token_file" 2>/dev/null; then
  echo "monacoctl dev token failed for actor A" >&2
  exit 2
fi

get() {
  curl -fsS -H "Authorization: Bearer $(tail -n 1 "$token_file")" "$api$1"
}

mine="$(get /v1/me/cabals)"
cabal_id="$(python3 -c '
import json, sys
rows = [r for r in json.load(sys.stdin) if r["name"].startswith("QA pot ")]
if not rows:
    sys.exit("no QA pot cabal in GET /v1/me/cabals")
newest = rows[0]["name"]
count = sum(1 for r in rows if r["name"] == newest)
if count != 1:
    sys.exit("%s is listed %d times" % (newest, count))
print(rows[0]["id"])
' <<<"$mine")"

get "/v1/cabals/$cabal_id" | python3 -c '
import json, sys
cabal = json.load(sys.stdin)
rules = cabal["rules"]
want = {"join_mode": "request", "voter_mode": "list", "threshold": "unanimous", "proposal_expiry_seconds": 3600}
wrong = {k: rules[k] for k in want if rules[k] != want[k]}
if wrong:
    sys.exit("rules are %s, want %s" % (wrong, want))
members = cabal["members"]
if cabal["member_count"] != 1 or len(members) != 1 or not members[0]["can_vote"]:
    sys.exit("want one voting member, got %s" % members)
print("%s: %s, one voting member" % (cabal["name"], json.dumps(rules, sort_keys=True)))
'
