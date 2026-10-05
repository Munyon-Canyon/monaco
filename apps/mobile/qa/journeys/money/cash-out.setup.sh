#!/usr/bin/env bash
set -euo pipefail

run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
scenario="${1:?journey.py passes the scenario id}"
# shellcheck source=/dev/null
source "$(git rev-parse --show-toplevel)/scripts/qa/seed.sh"
qa_api_ready

case "$scenario" in
  S1)
    name="QA cash out $run"
    qa_api A POST /v1/cabals \
      "{\"name\":\"$name\",\"join_mode\":\"open\",\"voter_mode\":\"all\",\"threshold\":\"majority\",\"proposal_expiry_seconds\":86400}" >/dev/null
    echo "seeded: A created the open cabal '$name'"
    ;;
  S2)
    echo "S2 starts from A's funded account balance; nothing to seed"
    ;;
  *)
    echo "no setup for scenario $scenario" >&2
    exit 1
    ;;
esac
