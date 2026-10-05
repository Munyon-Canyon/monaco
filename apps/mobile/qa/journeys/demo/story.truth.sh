#!/usr/bin/env bash
set -euo pipefail

run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
# shellcheck source=/dev/null
source "$(git rev-parse --show-toplevel)/scripts/qa/seed.sh"
a="$(_qa_account A privy_user_id)"
b="$(_qa_account B privy_user_id)"
status=0

for name in "QA duo $run" "QA story $run"; do
  members="$(qa_sql -F ' ' -v name="$name" -v a="$a" -v b="$b" <<<"SELECT count(*),
      count(*) FILTER (WHERE u.privy_user_id = :'a'), count(*) FILTER (WHERE u.privy_user_id = :'b')
    FROM cabal_members m JOIN cabals c ON c.id = m.cabal_id JOIN users u ON u.id = m.user_id WHERE c.name = :'name'")" ||
    { echo "database cannot be reached" >&2; exit 2; }
  case "$members" in
    "0 0 0") echo "ok: the run never created $name" ;;
    "1 1 0") echo "ok: $name has only A, so B never joined" ;;
    "2 1 1") echo "ok: A and B are the members of $name" ;;
    *)
      echo "$name members (all, A, B): got '$members', want '2 1 1'"
      status=1
      ;;
  esac
done
exit "$status"
