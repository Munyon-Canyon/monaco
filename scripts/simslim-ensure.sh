#!/usr/bin/env bash
# Make a lane or journey simulator persistently slim with SimSlim.
#   simslim-ensure.sh create <udid>   just created: run `simslim on`
#   simslim-ensure.sh check <udid>    before a run: `simslim verify`, re-run `simslim on` once if not slim
# Never fails: a missing simslim or a failed `on` warns on stderr and the sim stays stock.
# MONACO_NO_SIMSLIM=1 skips everything. Never boots a simulator; --preserve-boot-state
# returns a shut down simulator to shutdown.
set -uo pipefail

mode="${1:-}"
udid="${2:-}"
if [[ -z "$udid" || ( "$mode" != create && "$mode" != check ) ]]; then
  echo "usage: simslim-ensure.sh create|check <udid>" >&2
  exit 0
fi
if [[ "${MONACO_NO_SIMSLIM:-}" == 1 ]]; then
  exit 0
fi
if ! command -v simslim >/dev/null 2>&1; then
  echo "warning: SimSlim not installed. ${udid} stays a stock simulator (set MONACO_NO_SIMSLIM=1 to silence)." >&2
  exit 0
fi

# The repo's profile, not a per-user copy of it: a copy that slims away `siri` leaves a simulator
# whose dictation availability handler spins the app's main thread once a text field takes focus.
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
profile="${SIMSLIM_PROFILE:-$root/ci/profiles/base-slim.json}"

if [[ "$mode" == check ]] && simslim verify "$udid" --profile "$profile" >/dev/null 2>&1; then
  exit 0
fi
simslim on "$udid" --profile "$profile" --preserve-boot-state >/dev/null 2>&1 \
  || echo "warning: simslim on failed for ${udid}. using it without slim." >&2
exit 0
