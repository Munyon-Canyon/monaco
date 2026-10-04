#!/usr/bin/env bash
# Print the simulator agent QA and MobileBuildMCP drive. In a linked worktree (a lane),
# or with MONACO_SIM_UDID set, that is the lane's own simulator (scripts/lane-sim-udid.sh).
# In the primary checkout it is SIMSLIM_UDID, and this fails if that is unset or missing.
# Human just run/build/stop uses scripts/resolve-ios-sim.sh instead (stock fallback).
set -euo pipefail

rc=0
lane="$("$(dirname "${BASH_SOURCE[0]}")/lane-sim-udid.sh")" || rc=$?
if (( rc == 0 )); then
  printf '%s\n' "$lane"
  exit 0
elif (( rc != 3 )); then
  exit "$rc"
fi

if [[ -z "${SIMSLIM_UDID:-}" ]]; then
  echo "error: SIMSLIM_UDID is unset." >&2
  echo "Agent QA needs one gold simulator. Export SIMSLIM_UDID=<udid> (shell rc or plain .env)." >&2
  echo "just run / just build mobile / ./scripts/ios-sim use resolve-ios-sim.sh and fall back to a stock sim." >&2
  echo "See README: SimSlim (gold simulator)." >&2
  exit 1
fi

if ! xcrun simctl list devices available | grep -Fq "$SIMSLIM_UDID"; then
  echo "error: SIMSLIM_UDID=${SIMSLIM_UDID} is not an available simulator." >&2
  exit 1
fi

printf '%s\n' "$SIMSLIM_UDID"
