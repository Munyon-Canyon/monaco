#!/usr/bin/env bash
set -euo pipefail

run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
# shellcheck source=/dev/null
source "$(git rev-parse --show-toplevel)/scripts/qa/seed.sh"
a="$(_qa_account A privy_user_id)"
name="QA slice $run"

members="$(qa_sql -F ' ' -v name="$name" -v a="$a" <<<"SELECT count(*), count(*) FILTER (WHERE u.privy_user_id = :'a')
  FROM cabal_members m JOIN cabals c ON c.id = m.cabal_id JOIN users u ON u.id = m.user_id WHERE c.name = :'name'")" ||
  { echo "database cannot be reached" >&2; exit 2; }
if [[ "$members" == "0 0" ]]; then
  echo "ok: the run never created $name"
elif [[ "$members" == "1 1" ]]; then
  echo "ok: A is the only member of $name"
else
  echo "$name members (all, A): got '$members', want '1 1'"
  exit 1
fi
