#!/usr/bin/env bash
set -euo pipefail

run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
# shellcheck source=/dev/null
source "$(git rev-parse --show-toplevel)/scripts/qa/seed.sh"

name="QA chat $run"
a="$(_qa_account A privy_user_id)"
b="$(_qa_account B privy_user_id)"
members="$(qa_sql -F ' ' -v name="$name" -v a="$a" -v b="$b" <<<"SELECT count(*), count(*) FILTER (WHERE u.privy_user_id IN (:'a', :'b'))
  FROM cabal_members m JOIN cabals c ON c.id = m.cabal_id JOIN users u ON u.id = m.user_id WHERE c.name = :'name'")" ||
  { echo "database cannot be reached" >&2; exit 2; }
if [[ "$members" == "0 0" ]]; then
  echo "ok: the run never created $name"
elif [[ "$members" == "2 2" ]]; then
  echo "ok: $name has exactly A and B; no chat message table to read until #676"
else
  echo "$name members (all, A or B): got '$members', want '2 2'"
  exit 1
fi
