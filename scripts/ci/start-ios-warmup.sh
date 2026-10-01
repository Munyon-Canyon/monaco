#!/usr/bin/env bash
# Boot the simulator and start the Xcode and actool warm-up in the background.
# The warm-up uses its own DerivedData so it cannot lock the build.
# xcodebuild -list is not here: the toolchain probe showed it resolves packages.
# nohup keeps the warm-up alive after this step's shell exits. A later step
# cannot wait on that pid, because it is not that step's child, so the pid
# and the exit code are files.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$root"

sim="$(scripts/resolve-ios-sim.sh)"
xcrun simctl list devices available | grep -F "$sim"
xcrun simctl boot "$sim"
echo "SIM=$sim" >> "${GITHUB_ENV:?}"

mkdir -p "$RUNNER_TEMP/warmup" "$RUNNER_TEMP/actool-warm"
log="$RUNNER_TEMP/warmup.log"
rm -f "$RUNNER_TEMP/warmup-status"
date +%s > "$RUNNER_TEMP/warmup-started"
: > "$log"
nohup scripts/ci/ios-warmup-commands.sh >> "$log" 2>&1 &
echo $! > "$RUNNER_TEMP/warmup.pid"
echo "warmup pid $(cat "$RUNNER_TEMP/warmup.pid") sim $sim"
