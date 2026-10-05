#!/usr/bin/env bash
set -euo pipefail

run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
# shellcheck source=/dev/null
source "$(git rev-parse --show-toplevel)/scripts/qa/seed.sh"

name="QA bot $run"
got="$(qa_sql -v name="$name" <<<"SELECT
    (SELECT count(*) FROM cabal_members m JOIN cabals c ON c.id = m.cabal_id WHERE c.name = :'name') || ' ' ||
    (SELECT count(*) FROM proposals p JOIN cabals c ON c.id = p.cabal_id WHERE c.name = :'name')")" ||
  { echo "database cannot be reached" >&2; exit 2; }
read -r members proposals <<<"$got"
fail=0
if [[ "$members" -le 1 ]]; then
  echo "ok: $name has $members member(s)"
else
  echo "$name members: got $members, want A alone"
  fail=1
fi
if [[ "$proposals" -le 1 ]]; then
  echo "ok: $name has $proposals proposal(s), at most the one S2 sends"
else
  echo "$name proposals: got $proposals, want at most 1"
  fail=1
fi
exit "$fail"
