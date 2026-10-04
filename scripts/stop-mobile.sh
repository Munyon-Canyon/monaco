#!/usr/bin/env bash
# Stop this checkout's Monaco builds and app sessions, and no other worktree's.
#
# - xcodebuild: only processes building into this checkout's .build/DerivedData.
# - In a lane (a linked worktree): terminate and uninstall com.monaco.app on the lane's
#   simulators only ("Monaco <lane>", MONACO_SIM_UDID, and the lane's rows in the registry
#   scripts/lane-sim-udid.sh keeps).
# - In the primary checkout: terminate and uninstall on every available simulator except
#   those of lanes whose worktree still exists, and delete the simulators of lanes whose
#   worktree is gone.
# Never simctl erase.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

bundle_id="com.monaco.app"
top="$(git rev-parse --show-toplevel)"
git_dir="$(git rev-parse --path-format=absolute --git-dir)"
common_dir="$(git rev-parse --path-format=absolute --git-common-dir)"
registry="$common_dir/monaco-lane-sims.tsv"
available="$(xcrun simctl list devices available | grep -Eo '[A-F0-9-]{36}' | sort -u || true)"

stop_app() {
  local udid="$1"
  if xcrun simctl terminate "$udid" "$bundle_id" 2>/dev/null; then
    echo "terminated ${bundle_id} on ${udid}"
  fi
  if xcrun simctl uninstall "$udid" "$bundle_id" 2>/dev/null; then
    echo "uninstalled ${bundle_id} on ${udid} (cleared app container + Privy session)"
  fi
}

# Paths of the linked worktrees (the first entry is the primary checkout).
linked_worktrees() {
  git worktree list --porcelain | sed -n 's/^worktree //p' | tail -n +2
}

registry_rows() {
  if [[ -f "$registry" ]]; then
    grep -v '^$' "$registry" || true
  fi
}

if [[ "$git_dir" != "$common_dir" ]]; then
  lane="${top##*/}"
  targets="$(
    if [[ -n "${MONACO_SIM_UDID:-}" ]]; then echo "$MONACO_SIM_UDID"; fi
    xcrun simctl list devices available | grep -F "    Monaco ${lane} (" | grep -Eo '[A-F0-9-]{36}' || true
    registry_rows | awk -F'\t' -v l="$lane" '$2 == l { print $1 }'
  )"
  while IFS= read -r udid; do
    if [[ -n "$udid" ]] && grep -Fxq "$udid" <<< "$available"; then
      stop_app "$udid"
    fi
  done <<< "$(sort -u <<< "$targets")"
else
  live_lanes="$(linked_worktrees |
    while IFS= read -r path; do
      if [[ -d "$path" ]]; then echo "${path##*/}"; fi
    done)"
  # A live lane's simulators by name too, for any its registry row does not cover.
  skip="$(xcrun simctl list devices available -j | LIVE="$live_lanes" python3 -c '
import json, os, sys
lanes = [l for l in os.environ["LIVE"].splitlines() if l]
for ds in json.load(sys.stdin).get("devices", {}).values():
    for d in ds:
        n = d.get("name", "")
        if any(n == "Monaco " + l or n.startswith("Monaco Journeys " + l + " ") for l in lanes):
            print(d["udid"])
')"$'\n'
  kept=""
  while IFS=$'\t' read -r udid lane name; do
    [[ -n "$udid" ]] || continue
    if grep -Fxq "$lane" <<< "$live_lanes"; then
      skip+="$udid"$'\n'
      kept+="$udid"$'\t'"$lane"$'\t'"$name"$'\n'
    else
      if grep -Fxq "$udid" <<< "$available"; then
        xcrun simctl delete "$udid"
        skip+="$udid"$'\n'
        echo "deleted simulator ${name} (${udid}), its worktree ${lane} is gone"
      fi
    fi
  done <<< "$(registry_rows)"
  if [[ -f "$registry" ]]; then
    printf '%s' "$kept" > "$registry.tmp" && mv "$registry.tmp" "$registry"
  fi
  if [[ -z "$available" ]]; then
    echo "no available iOS simulators"
  fi
  while IFS= read -r udid; do
    if [[ -n "$udid" ]] && ! grep -Fxq "$udid" <<< "$skip"; then
      stop_app "$udid"
    fi
  done <<< "$available"
fi

derived_re="$(printf '%s' "$top/.build/DerivedData" | sed 's/[][\\.*^$+?(){}|]/\\&/g')"
pids="$(pgrep -f "xcodebuild.*${derived_re}" || true)"
if [[ -n "$pids" ]]; then
  # shellcheck disable=SC2086
  kill $pids 2>/dev/null || true
  echo "stopped xcodebuild for Monaco in ${top}"
fi

_ios_clipboard_bridge() {
  local cmd="$1"
  if command -v ios-sim-clipboard-bridge >/dev/null 2>&1; then
    ios-sim-clipboard-bridge "$cmd" || true
  elif [[ -x "${HOME}/.local/bin/ios-sim-clipboard-bridge" ]]; then
    "${HOME}/.local/bin/ios-sim-clipboard-bridge" "$cmd" || true
  fi
}
_ios_clipboard_bridge stop
