#!/usr/bin/env bash
set -euo pipefail

run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
# shellcheck source=/dev/null
source "$(git rev-parse --show-toplevel)/scripts/qa/seed.sh"

fail=0
items() {
  qa_sql -v name="$1" -v kind="$2" <<<"SELECT count(*) FROM feed_objects WHERE cabal_name = :'name' AND kind = :'kind'" ||
    { echo "database cannot be reached" >&2; exit 2; }
}
check() {
  local got
  got="$(items "$1" "$2")"
  if [[ "$got" == 0 ]]; then
    echo "ok: the run never created $1"
  elif [[ "$got" == 1 ]]; then
    echo "ok: $1 has one $2 item"
  else
    echo "$1 $2 items: got $got, want 1"
    fail=1
  fi
}

check "QA feed $run" cabal_created
check "QA feed $run" member_joined
check "QA own $run" cabal_created

a="$(qa_user_id A)"
b="$(qa_user_id B)"
follows="$(qa_sql -v a="$a" -v b="$b" <<<"SELECT count(*) FROM follows WHERE follower_id = :'a' AND followee_id = :'b' AND deleted_at IS NULL")"
if [[ "$follows" != 0 ]]; then
  echo "A follows B after browsing the feed; browsing must not follow anyone"
  fail=1
else
  echo "ok: A does not follow B"
fi
exit "$fail"
