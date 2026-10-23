#!/usr/bin/env bash
# Run MonacoTests on the simulator start-ios-sim.sh picked, with a per-attempt timeout.
# A hung xcodebuild test-without-building, as in run 39899096438 which sat 13 minutes
# after Device already booted, is killed after MONACO_IOS_TEST_TIMEOUT seconds, default 720.
# The simulator is restarted and the tests run once more. A second expiry fails.
# Env: RUNNER_TEMP, DERIVED_DATA. MONACO_IOS_TEST_CMD, MONACO_IOS_TEST_TIMEOUT and
# MONACO_IOS_XCRUN exist for scripts/ci/run_monaco_tests_test.go.
set -euo pipefail

temp="${RUNNER_TEMP:?}"
limit="${MONACO_IOS_TEST_TIMEOUT:-720}"
xcrun_bin="${MONACO_IOS_XCRUN:-xcrun}"

for _ in $(seq "${MONACO_IOS_SIM_WAIT:-180}"); do [[ -s "$temp/sim" ]] && break; sleep 1; done
if [[ ! -s "$temp/sim" ]]; then
  cat "$temp/simctl-boot.log" 2>/dev/null || true
  exit 1
fi
SIM="$(cat "$temp/sim")"
export SIM
"$xcrun_bin" simctl bootstatus "$SIM" -b

# shellcheck disable=SC2016
default_cmd='xcodebuild test-without-building -project apps/mobile/Monaco.xcodeproj -scheme Monaco -testPlan MonacoTests -configuration Debug \
  -destination "platform=iOS Simulator,id=$SIM" -derivedDataPath "$DERIVED_DATA" \
  -resultBundlePath "$RUNNER_TEMP/MonacoTests.xcresult" CODE_SIGNING_ALLOWED=NO'
cmd="${MONACO_IOS_TEST_CMD:-$default_cmd}"

attempt() {
  rm -rf "$temp/MonacoTests.xcresult" "$temp/hung"
  bash -c "$cmd" &
  local pid=$!
  (
    trap 'kill "$nap" 2>/dev/null; exit 0' TERM
    sleep "$limit" &
    nap=$!
    wait "$nap"
    touch "$temp/hung"
    kill "$pid" 2>/dev/null
  ) &
  local dog=$!
  local code=0
  wait "$pid" || code=$?
  kill "$dog" 2>/dev/null || true
  wait "$dog" 2>/dev/null || true
  [[ -e "$temp/hung" ]] && return 142
  return "$code"
}

code=0
attempt || code=$?
if [[ "$code" == 142 ]]; then
  echo "MonacoTests hung for 12 minutes, shutting the simulator down and retrying once"
  "$xcrun_bin" simctl shutdown "$SIM" || true
  "$xcrun_bin" simctl boot "$SIM"
  "$xcrun_bin" simctl bootstatus "$SIM" -b
  code=0
  attempt || code=$?
  if [[ "$code" == 142 ]]; then
    echo "MonacoTests hung for 12 minutes a second time, failing the step"
  fi
fi
exit "$code"
