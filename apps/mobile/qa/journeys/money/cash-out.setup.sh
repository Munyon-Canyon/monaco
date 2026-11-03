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
    created="$(qa_api A POST /v1/cabals \
      "{\"name\":\"$name\",\"join_mode\":\"request\",\"voter_mode\":\"all\",\"threshold\":\"majority\",\"proposal_expiry_seconds\":86400}")"
    id="$(jq -r .id <<<"$created")"
    echo "seeded: A created the cabal '$name'"
    funded="$(qa_api A POST "/v1/cabals/$id/fund" '{"amount_micros":"1000000"}')"
    transfer="$(jq -r .transfer_id <<<"$funded")"
    echo "seeded: A funded 1 USDC, transfer $transfer"
    status=""
    for _ in $(seq 60); do
      body="$(qa_api A GET "/v1/fund-transfers/$transfer")"
      status="$(jq -r .status <<<"$body")"
      case "$status" in
        settled) break ;;
        failed)
          echo "fund transfer $transfer failed: $(jq -r .fail_code <<<"$body")" >&2
          exit 1
          ;;
      esac
      sleep 2
    done
    if [ "$status" != settled ]; then
      echo "fund transfer $transfer still $status after 120 s" >&2
      exit 1
    fi
    echo "seeded: fund transfer $transfer settled"
    ;;
  S2)
    echo "S2 starts from A's funded account balance; nothing to seed"
    ;;
  *)
    echo "no setup for scenario $scenario" >&2
    exit 1
    ;;
esac
