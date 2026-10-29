#!/usr/bin/env bash
set -euo pipefail

scenario="${1:?journey.py passes the scenario id}"
handoff="${MONACO_QA_HANDOFF:?journey.py sets MONACO_QA_HANDOFF}"
# shellcheck source=/dev/null
source "$(git rev-parse --show-toplevel)/scripts/qa/seed.sh"
qa_api_ready

case "$scenario" in
  S1) ;;
  S2 | S3) exit 0 ;;
  *)
    echo "no setup for scenario $scenario" >&2
    exit 1
    ;;
esac

did="$(_qa_account A privy_user_id)"
qa_sql -v did="$did" >/dev/null \
  <<<"UPDATE users SET auth_state = 'ONBOARDING_COMPLETED', auth_state_changed_at = now() WHERE privy_user_id = :'did' AND auth_state <> 'ONBOARDING_COMPLETED'"
qa_api A PATCH /v1/me "{\"display_name\":\"$(_qa_account A name)\"}" >/dev/null
me="$(qa_user_id A)"

# F19aResolveCallsOK seeds eight voters with 5, 8, 7, 6, 5, 5, 5 and 2 resolved
# votes this month under fresh users. A takes over the seventh voter's votes
# (7 resolved, 4 right, below the top three) and user_7 stays the one under the
# five-vote floor. Three things keep a shared database from skewing that:
# A's call_voters rows from earlier runs are deleted first, the seeded users get
# display names (the board copies them, and the seed has none), and a board
# rebuild is triggered and awaited so Home and the Leaderboard include A.
seed="$(qa_flow_seed 19a ok)"
read -r voter unranked < <(python3 -c 'import json,sys; ids=json.load(sys.stdin)["ids"]; print(ids["user_2"], ids["user_7"])' <<<"$seed")
qa_sql -v me="$me" -v voter="$voter" >/dev/null <<'SQL'
DELETE FROM call_voters WHERE user_id = :'me';
UPDATE call_voters SET user_id = :'me' WHERE user_id = :'voter';
SQL

k=0
while read -r uid; do
  k=$((k + 1))
  qa_sql -v uid="$uid" -v name="QA caller $k" >/dev/null \
    <<<"UPDATE users SET display_name = :'name' WHERE id = :'uid'"
done < <(python3 -c 'import json,sys; ids=json.load(sys.stdin)["ids"]; print(*[v for k, v in ids.items() if k.startswith("user_") and v != sys.argv[1]], sep="\n")' "$voter" <<<"$seed")

# The seed's cabal has calls but no cabals row, and the Cabals board ranks only cabals rows.
cabal="$(python3 -c 'import json,sys; print(json.load(sys.stdin)["ids"]["cabal"])' <<<"$seed")"
qa_sql -v cabal="$cabal" -v me="$me" >/dev/null <<'SQL'
INSERT INTO cabals (id, name, creator_id, join_mode, voter_mode, threshold, proposal_expiry_seconds, created_at, updated_at)
VALUES (:'cabal', 'QA called cabal', :'me', 'request', 'all', 'majority', 86400, now(), now())
ON CONFLICT (id) DO NOTHING;
SQL

triggered="$(qa_sql <<<"INSERT INTO call_board_triggers (reason, created_at) VALUES ('qa_journey', now()) RETURNING created_at")"
triggered="${triggered%%$'\n'*}"
rebuilt=
for _ in $(seq 20); do
  rebuilt="$(qa_sql -v t="$triggered" <<<"SELECT count(*) FROM call_board_runs WHERE started_at > :'t'::timestamptz")"
  [[ "$rebuilt" -gt 0 ]] && break
  sleep 1
done
if [[ "${rebuilt:-0}" -eq 0 ]]; then
  echo "the ranking.call_board poller did not rebuild the board within 20 s of the trigger at $triggered: is the worker running?" >&2
  exit 1
fi

python3 - "$handoff" "$me" "$unranked" <<'PY'
import json, pathlib, sys
path = pathlib.Path(sys.argv[1])
values = json.loads(path.read_text()) if path.exists() and path.read_text().strip() else {}
values.update(meID=sys.argv[2], unrankedID=sys.argv[3])
path.write_text(json.dumps(values))
PY
echo "seeded: F19aResolveCallsOK, A holds the votes of its third voter, $unranked has two resolved votes"
