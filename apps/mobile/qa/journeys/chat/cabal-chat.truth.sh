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
  exit 0
elif [[ "$members" != "2 2" ]]; then
  echo "$name members (all, A or B): got '$members', want '2 2'"
  exit 1
fi
echo "ok: $name has exactly A and B"
ballots="$(qa_sql -F ' ' -v name="$name" -v b="$b" <<<"SELECT count(*), count(*) FILTER (WHERE u.privy_user_id = :'b' AND v.choice = 'yes')
  FROM votes v JOIN proposals p ON p.id = v.proposal_id JOIN cabals c ON c.id = p.cabal_id JOIN users u ON u.id = v.voter_id
  WHERE c.name = :'name'")" ||
  { echo "database cannot be reached" >&2; exit 2; }
if [[ "$ballots" == "0 0" || "$ballots" == "1 1" ]]; then
  echo "ok: the seeded proposal has no ballot or only B's yes ($ballots)"
else
  echo "$name ballots (all, B's yes): got '$ballots', want '0 0' or '1 1'"
  exit 1
fi
