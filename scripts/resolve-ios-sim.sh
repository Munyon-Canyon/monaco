#!/usr/bin/env bash
# Print one iOS Simulator UDID on stdout.
# In a linked worktree (a lane), or with MONACO_SIM_UDID set, print the lane's own
# simulator (scripts/lane-sim-udid.sh). In the primary checkout:
# prefer SIMSLIM_UDID when that device exists. If SimSlim / PATH ios-sim is missing
# or not ready, warn on stderr and still use that UDID as a stock sim, or pick another.
# Never commits a UDID. Never simctl erase.
set -euo pipefail

warn() {
  echo "warning: $*" >&2
}

simctl() {
  xcrun simctl "$@"
}

device_exists() {
  local udid="$1"
  simctl list devices available | grep -Fq "$udid"
}

slim_ready() {
  local udid="$1"
  if ! command -v simslim >/dev/null 2>&1; then
    return 1
  fi
  local profile="${SIMSLIM_PROFILE:-}"
  if [[ -z "$profile" && -f "${HOME}/.config/simslim/base-slim.json" ]]; then
    profile="${HOME}/.config/simslim/base-slim.json"
  fi
  if [[ -n "$profile" ]]; then
    simslim verify "$udid" --profile "$profile" >/dev/null 2>&1
  else
    simslim verify "$udid" >/dev/null 2>&1
  fi
}

# Pick an available iOS simulator when no UDID is set. Devices are ranked by name:
# 0 "iPhone <n> Pro Max", 1 "iPhone <n> Pro", 2 "iPhone <n>" and "iPhone <n> Plus",
# 3 "iPhone Air" and "iPhone <n>e", 4 any other iPhone, 5 iPhone Duo (Monaco is not
# designed for it), 6 anything that is not an iPhone. The lowest rank wins; inside a
# rank a booted device beats a shut-down one, then the newest runtime wins. Duo and
# non-iPhones are picked only when no other iPhone exists on any runtime.
pick_available_udid() {
  if command -v python3 >/dev/null 2>&1; then
    simctl list devices available -j | python3 -c '
import json, re, sys
data = json.load(sys.stdin)

def runtime_version(runtime):
    # com.apple.CoreSimulator.SimRuntime.iOS-26-0 -> (26, 0)
    return tuple(int(n) for n in re.findall(r"\d+", runtime.rsplit("iOS", 1)[-1]))

def rank(name):
    if not name.startswith("iPhone"):
        return 6
    if "Duo" in name:
        return 5
    if re.fullmatch(r"iPhone \d+ Pro Max", name):
        return 0
    if re.fullmatch(r"iPhone \d+ Pro", name):
        return 1
    if re.fullmatch(r"iPhone \d+( Plus)?", name):
        return 2
    if re.fullmatch(r"iPhone Air|iPhone \d+e", name):
        return 3
    return 4

best = None
for runtime, devices in data.get("devices", {}).items():
    if "iOS" not in runtime:
        continue
    version = runtime_version(runtime)
    for dev in devices:
        udid = dev.get("udid") or ""
        if not udid or not dev.get("isAvailable", True):
            continue
        name = dev.get("name") or ""
        key = (rank(name), dev.get("state") != "Booted", tuple(-n for n in version))
        if best is None or key < best[0]:
            best = (key, udid)
print(best[1] if best else "")
'
    return 0
  fi
  simctl list devices available | awk '
    /^-- iOS [0-9.]+ --$/ { split($3, v, "."); version = v[1] * 1000 + v[2]; inios = 1; next }
    /^-- / { inios = 0; next }
    !inios { next }
    {
      if (!match($0, /\([A-F0-9-]+\) \([A-Za-z ]+\)[ ]*$/)) next
      tail = substr($0, RSTART)
      name = substr($0, 1, RSTART - 1)
      sub(/^[ ]+/, "", name)
      sub(/[ ]+$/, "", name)
      udid = tail; sub(/^\(/, "", udid); sub(/\).*$/, "", udid)
      booted = (tail ~ /\(Booted\)[ ]*$/) ? 0 : 1
      if (name !~ /^iPhone/) rank = 6
      else if (name ~ /Duo/) rank = 5
      else if (name ~ /^iPhone [0-9]+ Pro Max$/) rank = 0
      else if (name ~ /^iPhone [0-9]+ Pro$/) rank = 1
      else if (name ~ /^iPhone [0-9]+( Plus)?$/) rank = 2
      else if (name ~ /^iPhone Air$/ || name ~ /^iPhone [0-9]+e$/) rank = 3
      else rank = 4
      score = rank * 10000000 + booted * 1000000 + (999999 - version)
      if (best == "" || score < bestscore) { best = udid; bestscore = score }
    }
    END { print best }
  '
}

emit() {
  printf '%s\n' "$1"
}

# MONACO_LANE_LOOKUP=off is how lane-sim-udid.sh asks for the primary checkout's pick.
if [[ "${MONACO_LANE_LOOKUP:-}" != "off" ]]; then
  rc=0
  lane="$("$(dirname "${BASH_SOURCE[0]}")/lane-sim-udid.sh")" || rc=$?
  if (( rc == 0 )); then
    emit "$lane"
    exit 0
  elif (( rc != 3 )); then
    exit "$rc"
  fi
fi

if [[ -n "${SIMSLIM_UDID:-}" ]]; then
  if device_exists "$SIMSLIM_UDID"; then
    if slim_ready "$SIMSLIM_UDID"; then
      emit "$SIMSLIM_UDID"
      exit 0
    fi
    warn "SimSlim not installed or not ready for ${SIMSLIM_UDID}. using that simulator without slim."
    emit "$SIMSLIM_UDID"
    exit 0
  fi
  warn "SIMSLIM_UDID=${SIMSLIM_UDID} is not an available simulator. picking another."
fi

if ! command -v simslim >/dev/null 2>&1; then
  warn "SimSlim not installed. falling back to a stock Xcode simulator. optional setup is in README (SimSlim)."
elif [[ -z "${SIMSLIM_UDID:-}" ]]; then
  warn "SIMSLIM_UDID unset. falling back to a stock Xcode simulator."
fi

picked="$(pick_available_udid)"
if [[ -z "$picked" ]]; then
  echo "error: no iOS Simulator available. Open Xcode > Settings > Platforms, download an iOS 18+ runtime, then create an iPhone simulator." >&2
  exit 1
fi
emit "$picked"
