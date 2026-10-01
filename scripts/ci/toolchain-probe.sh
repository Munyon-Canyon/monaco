#!/usr/bin/env bash
# Time the first Xcode, CoreSimulator and actool launches on a fresh runner.
# Prints one "probe <label>: <milliseconds>ms" line per command. Does not build.
set -euo pipefail

order="${PROBE_ORDER:-pinned-first}"
case "$order" in
  pinned-first | default-first) ;;
  *)
    echo "probe-order must be pinned-first or default-first, got '${order}'" >&2
    exit 1
    ;;
esac

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$root"

temp="${RUNNER_TEMP:-${TMPDIR:-/tmp}}"
probe_derived="$temp/probe"
default_developer="/Applications/Xcode_26.6.app/Contents/Developer"
catalog="apps/mobile/Monaco/Assets.xcassets"
times="$temp/probe-times.tsv"
: > "$times"
mkdir -p "$probe_derived"

now_ms() {
  python3 -c 'import time; print(int(time.time() * 1000))'
}

step_start=$(now_ms)

echo "probe order: $order"
echo "Select Xcode already ran xcodebuild -version on the pinned toolchain before this step."
echo "default developer dir: $default_developer"

time_cmd() {
  local label="$1"
  shift
  local start elapsed
  start=$(now_ms)
  echo "=== probe: ${label} ==="
  "$@"
  elapsed=$(( $(now_ms) - start ))
  echo "probe ${label}: ${elapsed}ms"
  printf '%s\t%s\n' "$label" "$elapsed" >> "$times"
}

pinned_showsdks() {
  unset DEVELOPER_DIR
  echo "pinned developer dir: $(xcode-select -p)"
  xcodebuild -showsdks
}

default_showsdks() {
  if [[ ! -x "$default_developer/usr/bin/xcodebuild" ]]; then
    echo "default Xcode missing at ${default_developer}" >&2
    ls -d /Applications/Xcode*.app >&2 || true
    return 1
  fi
  DEVELOPER_DIR="$default_developer" xcodebuild -showsdks
}

list_project() {
  local log="$temp/probe-list.log"
  # -derivedDataPath is rejected unless -scheme, -testProductsPath, or -xctestrun is set.
  xcodebuild -list -project apps/mobile/Monaco.xcodeproj -scheme Monaco -derivedDataPath "$probe_derived" 2>&1 | tee "$log"
  local resolved
  resolved="$(scripts/ci/probe-list-resolved.sh "$log" "$probe_derived")"
  echo "probe list started package resolution: ${resolved}"
}

simctl_list() {
  xcrun simctl list devices -j
}

actool_compile() {
  local n="$1"
  local out="$temp/actool-${n}"
  rm -rf "$out"
  mkdir -p "$out"
  xcrun actool --compile "$out" --platform iphonesimulator --minimum-deployment-target 18.0 "$catalog"
}

if [[ "$order" == "default-first" ]]; then
  time_cmd default-showsdks default_showsdks
  time_cmd pinned-showsdks pinned_showsdks
else
  time_cmd pinned-showsdks pinned_showsdks
  time_cmd default-showsdks default_showsdks
fi
echo "pinned version: $(xcodebuild -version | tr '\n' ' ')"
echo "default version: $(DEVELOPER_DIR="$default_developer" xcodebuild -version | tr '\n' ' ')"

time_cmd xcodebuild-list list_project
time_cmd simctl-list simctl_list
time_cmd actool-1 actool_compile 1
time_cmd actool-2 actool_compile 2

elapsed=$(( $(now_ms) - step_start ))
echo "probe total: ${elapsed}ms"
if [[ -n "${RUNNER_TEMP:-}" ]]; then
  printf 'Toolchain probe\t%s\n' "$(( (elapsed + 500) / 1000 ))" >> "$RUNNER_TEMP/step-times.tsv"
fi
if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
  {
    echo "### Toolchain probe"
    echo
    echo '```'
    echo "order: ${order}"
    cat "$times"
    echo "total: ${elapsed}"
    echo '```'
    echo
  } >> "$GITHUB_STEP_SUMMARY"
fi
