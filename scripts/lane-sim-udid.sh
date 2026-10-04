#!/usr/bin/env bash
# Print this lane's simulator UDID. A lane is a linked git worktree, named by its
# directory; its simulator is "Monaco <lane>", created on first use with the gold
# simulator's device type and runtime, so agents in separate worktrees never share one.
#
# Each simulator a lane creates is recorded as `<udid>\t<lane>\t<name>` in
# monaco-lane-sims.tsv in the git common dir.
#
# MONACO_SIM_UDID, when set, wins everywhere (lane or primary) and is printed as is.
# Exit 3 (nothing printed) means this checkout is the primary one, not a lane: callers
# then fall back to SIMSLIM_UDID or a stock simulator (scripts/resolve-ios-sim.sh).
# Never commits a UDID. Never simctl erase.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

if [[ -n "${MONACO_SIM_UDID:-}" ]]; then
  printf '%s\n' "$MONACO_SIM_UDID"
  exit 0
fi

git_dir="$(git rev-parse --path-format=absolute --git-dir)"
common_dir="$(git rev-parse --path-format=absolute --git-common-dir)"
if [[ "$git_dir" == "$common_dir" ]]; then
  exit 3
fi
top="$(git rev-parse --show-toplevel)"
name="Monaco ${top##*/}"

# UDID of the available simulator with this exact name; the lowest UDID if there are several.
find_by_name() {
  xcrun simctl list devices available -j | python3 -c '
import json, sys
name = sys.argv[1]
found = sorted(d["udid"] for ds in json.load(sys.stdin).get("devices", {}).values()
               for d in ds if d.get("name") == name and d.get("isAvailable", True))
print(found[0] if found else "")
' "$name"
}

udid="$(find_by_name)"
if [[ -n "$udid" ]]; then
  printf '%s\n' "$udid"
  exit 0
fi

# One creator per lane at a time (mkdir is atomic); a lock whose pid is gone is stale.
lock="$top/.build/lane-sim.lock"
mkdir -p "$top/.build"
have_lock=0
trap 'if (( have_lock )); then rm -rf "$lock"; fi' EXIT
tries=0
while (( tries < 600 )); do
  tries=$((tries + 1))
  if mkdir "$lock" 2>/dev/null; then
    have_lock=1
    echo "$$" > "$lock/pid"
    break
  fi
  owner="$(cat "$lock/pid" 2>/dev/null || true)"
  if [[ -n "$owner" ]] && ! kill -0 "$owner" 2>/dev/null; then
    rm -rf "$lock"
    continue
  fi
  sleep 0.2
done
if (( ! have_lock )); then
  echo "error: timed out waiting for ${lock}" >&2
  exit 1
fi

udid="$(find_by_name)"
if [[ -n "$udid" ]]; then
  printf '%s\n' "$udid"
  exit 0
fi

# The gold simulator is whatever the primary checkout would pick.
gold="$(MONACO_LANE_LOOKUP=off ./scripts/resolve-ios-sim.sh 2>/dev/null)"
template="$(xcrun simctl list devices available -j | python3 -c '
import json, sys
gold = sys.argv[1]
for runtime, ds in json.load(sys.stdin).get("devices", {}).items():
    for d in ds:
        if d.get("udid") == gold:
            print(d["deviceTypeIdentifier"], runtime)
' "$gold")"
if [[ -z "$template" ]]; then
  echo "error: no gold simulator to copy a device type and runtime from" >&2
  exit 1
fi
read -r device_type runtime <<< "$template"
udid="$(xcrun simctl create "$name" "$device_type" "$runtime")"
echo "created simulator \"${name}\" (${udid}) from ${device_type} ${runtime}" >&2
# The registry is how scripts/stop-mobile.sh in the primary checkout tells lane simulators
# apart from the operator's own and deletes them once their worktree is gone.
printf '%s\t%s\t%s\n' "$udid" "${top##*/}" "$name" >> "$common_dir/monaco-lane-sims.tsv"

if command -v simslim >/dev/null 2>&1; then
  profile="${SIMSLIM_PROFILE:-}"
  if [[ -z "$profile" && -f "${HOME}/.config/simslim/base-slim.json" ]]; then
    profile="${HOME}/.config/simslim/base-slim.json"
  fi
  if [[ -n "$profile" ]]; then
    simslim on "$udid" --profile "$profile" >/dev/null 2>&1 \
      || echo "warning: simslim on failed for ${udid}. using it without slim." >&2
  fi
fi

printf '%s\n' "$udid"
