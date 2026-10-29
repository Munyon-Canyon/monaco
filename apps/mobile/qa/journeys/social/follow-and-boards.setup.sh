#!/usr/bin/env bash
set -euo pipefail

scenario="${1:?journey.py passes the scenario id}"
: "${MONACO_QA_HANDOFF:?journey.py sets MONACO_QA_HANDOFF}"
# shellcheck source=/dev/null
source "$(git rev-parse --show-toplevel)/scripts/qa/seed.sh"
qa_api_ready

hand_off() {
  python3 - "$MONACO_QA_HANDOFF" "$1" "$2" <<'PY'
import json, os, sys
path, key, value = sys.argv[1:]
values = json.load(open(path)) if os.path.exists(path) else {}
values[key] = value
json.dump(values, open(path, "w"))
PY
}

ready_actor() {
  local actor="$1" did name
  did="$(_qa_account "$actor" privy_user_id)"
  qa_sql -v did="$did" >/dev/null \
    <<<"UPDATE users SET auth_state = 'ONBOARDING_COMPLETED', auth_state_changed_at = now() WHERE privy_user_id = :'did' AND auth_state <> 'ONBOARDING_COMPLETED'"
  name="$(_qa_account "$actor" name)"
  qa_api "$actor" PATCH /v1/me "{\"display_name\":\"$name\"}" >/dev/null
}

unfollow() {
  local follower="$1" followee="$2" token
  token="$(_qa_monacoctl dev token --user "$follower" --ttl 1h </dev/null)"
  curl -fsS -o /dev/null -X DELETE "$QA_API/v1/users/$followee/follow" \
    -H "Authorization: Bearer $token" -H "Content-Type: application/json" \
    -H "Idempotency-Key: $(uuidgen | tr '[:upper:]' '[:lower:]')" </dev/null
}

live_follows() {
  qa_sql -F ' ' -v a="$1" -v b="$2" <<<"SELECT follower_id, followee_id FROM follows
    WHERE deleted_at IS NULL AND (follower_id IN (:'a', :'b') OR followee_id = :'b')"
}

member_a="$(qa_user_id A)"
member_b="$(qa_user_id B)"
hand_off meID "$member_a"
hand_off meName "$(_qa_account A name)"
hand_off memberID "$member_b"
hand_off memberName "$(_qa_account B name)"

case "$scenario" in
  S1)
    ready_actor A
    ready_actor B
    # F19aResolveCallsOK seeds eight voters with 5, 8, 7, 6, 5, 5, 5 and 2 resolved votes
    # this month under fresh users. A takes the third voter's votes (7 resolved, 4 right) and
    # B the seventh's (5 resolved, 2 right), so both rank with five or more and neither is in
    # the top three, which the page draws without a leaderboard-row identifier.
    seed="$(qa_flow_seed 19a ok)"
    read -r voter_a voter_b < <(python3 -c 'import json,sys; ids=json.load(sys.stdin)["ids"]; print(ids["user_2"], ids["user_6"])' <<<"$seed")
    qa_sql -v a="$member_a" -v va="$voter_a" -v b="$member_b" -v vb="$voter_b" >/dev/null <<'SQL'
DELETE FROM call_voters WHERE user_id IN (:'a', :'b');
UPDATE call_voters SET user_id = CASE user_id WHEN :'va' THEN :'a'::uuid ELSE :'b'::uuid END WHERE user_id IN (:'va', :'vb');
SQL
    k=0
    while read -r uid; do
      k=$((k + 1))
      qa_sql -v uid="$uid" -v name="QA caller $k" >/dev/null \
        <<<"UPDATE users SET display_name = :'name' WHERE id = :'uid'"
    done < <(python3 -c 'import json,sys; ids=json.load(sys.stdin)["ids"]; print(*[v for k, v in ids.items() if k.startswith("user_") and v not in sys.argv[1:]], sep="\n")' "$voter_a" "$voter_b" <<<"$seed")
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
    while read -r follower followee; do
      [[ -n "$follower" ]] && unfollow "$follower" "$followee"
    done < <(live_follows "$member_a" "$member_b")
    if [[ -n "$(live_follows "$member_a" "$member_b")" ]]; then
      echo "A and B still have live follows after the unfollows" >&2
      exit 1
    fi
    hand_off followRowsBefore "$(qa_sql -v a="$member_a" -v b="$member_b" \
      <<<"SELECT count(*) FROM follows WHERE follower_id = :'a' AND followee_id = :'b'")"
    echo "seeded: F19aResolveCallsOK, A and B hold seeded resolved votes and rank, A and B follow nobody, B has no followers"
    ;;
  S4)
    hand_off liveAfterFollow "$(qa_sql -v a="$member_a" -v b="$member_b" \
      <<<"SELECT count(*) FROM follows WHERE follower_id = :'a' AND followee_id = :'b' AND deleted_at IS NULL")"
    echo "recorded: the live follows from A to B after S3"
    ;;
  S2 | S3 | S5) ;;
  *)
    echo "no setup for scenario $scenario" >&2
    exit 1
    ;;
esac
