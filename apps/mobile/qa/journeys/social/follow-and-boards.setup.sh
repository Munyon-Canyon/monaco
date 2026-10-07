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
    _qa_monacoctl seed two-cabals-ranked --users "$member_a,$member_b"
    while read -r follower followee; do
      [[ -n "$follower" ]] && unfollow "$follower" "$followee"
    done < <(live_follows "$member_a" "$member_b")
    if [[ -n "$(live_follows "$member_a" "$member_b")" ]]; then
      echo "A and B still have live follows after the unfollows" >&2
      exit 1
    fi
    hand_off followRowsBefore "$(qa_sql -v a="$member_a" -v b="$member_b" \
      <<<"SELECT count(*) FROM follows WHERE follower_id = :'a' AND followee_id = :'b'")"
    echo "seeded: A and B rank on two-cabals-ranked, A and B follow nobody, B has no followers"
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
