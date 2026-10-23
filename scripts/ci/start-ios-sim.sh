#!/usr/bin/env bash
# Pick the iOS simulator, write its UDID to <dir>/sim, then boot it.
# ci-ios.yml runs this in the background so the boot overlaps the cache restore
# and the build; the test step waits for <dir>/sim and then for the boot.
set -euo pipefail

if [[ $# -ne 1 || -z "$1" ]]; then
  echo "usage: start-ios-sim.sh <dir>" >&2
  exit 2
fi
dir="$1"
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

sim="$("$root/scripts/resolve-ios-sim.sh")"
xcrun simctl list devices -j | python3 -c '
import json, sys
udid = sys.argv[1]
for runtime, devices in json.load(sys.stdin)["devices"].items():
    for dev in devices:
        if dev.get("udid") == udid:
            print("simulator: %s on %s" % (dev.get("name"), runtime.rsplit(".", 1)[-1]))
' "$sim" || true
echo "$sim" > "$dir/sim.tmp"
mv "$dir/sim.tmp" "$dir/sim"
xcrun simctl boot "$sim"
