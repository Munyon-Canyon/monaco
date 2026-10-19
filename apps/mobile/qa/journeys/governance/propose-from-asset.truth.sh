#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
# shellcheck disable=SC1091
source scripts/qa/seed.sh
name="QA asset $run"

if [[ "$(qa_sql -v name="$name" <<<"SELECT count(*) FROM cabals WHERE name = :'name'")" != 1 ]]; then
  echo "skip: the run never created $name"
  exit 0
fi

got="$(qa_sql -v name="$name" <<<"SELECT count(*) FROM proposals p JOIN cabals c ON c.id = p.cabal_id WHERE c.name = :'name'")"
if [[ "$got" != 0 ]]; then
  echo "$name has $got proposals, want 0: the empty pot refuses the amount"
  exit 1
fi
echo "ok: $name has no proposal"
