#!/usr/bin/env bash
# Screenshot every Debug sample-harness screen listed in scripts/qa/sample-screens.txt.
#
#   scripts/qa/screens.sh --check                       # manifest covers every harness?
#   scripts/qa/screens.sh <sim udid> <Monaco.app> <dir> # install, launch each screen, shoot
#
# The manifest is the only list of screens. --check reads the app sources and fails both ways.
# Forward: a `-Monaco…Sample`/`…Gallery` or `-…Harness` launch flag, or a scenario of a
# `…Sample…` enum in a file that declares a SampleHarnessEntry, has no manifest line. Reverse: a
# manifest line (outside the generated block) launches a flag no harness file reads, a scenario
# that file does not know, or a further -Monaco flag no app source reads, so the shot would be
# the login screen. It also fails when a `-MonacoFlow <id> <outcome>` line of the generated block
# has no SampleHarnessEntry that reads Flow<id>Scenario.matching. The Journeys CI job runs it.
# Capture runs the check too and exits 1 when a screen failed to launch or shoot, or the check
# failed.
#
# MONACO_QA_SCREEN_SETTLE: seconds to wait after launch before the shot (default 4).
set -uo pipefail

root="$(cd "$(dirname "$0")/../.." && pwd)"
manifest="$root/scripts/qa/sample-screens.txt"
app_src="$root/apps/mobile/Monaco"
core_src="$root/packages/mobile-core/Sources"
settle="${MONACO_QA_SCREEN_SETTLE:-4}"

entries() { # name<TAB>args, comments and blank lines dropped
  sed -E 's/#.*//' "$manifest" | awk 'NF { name=$1; $1=""; sub(/^ +/, ""); print name "\t" $0 }'
}

in_manifest() { # every argument appears, in order, on one manifest line
  local pattern="(^|[[:space:]])$1"; shift
  local arg
  for arg in "$@"; do pattern="${pattern}[[:space:]]+${arg}"; done
  entries | cut -f2 | grep -qE "$pattern([[:space:]]|\$)"
}

# Cases of `enum …Sample…: String, CaseIterable` in one file, as their launch values.
enum_cases() {
  awk '
    /enum [A-Za-z0-9_]*Sample[A-Za-z0-9_]*: String, CaseIterable/ { inside=1; depth=0 }
    inside {
      line=$0
      before=depth
      opens=gsub(/\{/, "{", line); closes=gsub(/\}/, "}", line)
      depth += opens - closes
      if ($1 == "case") if (before == 1) {
        sub(/^[[:space:]]*case[[:space:]]+/, "")
        n=split($0, parts, ",")
        for (i=1; i<=n; i++) {
          c=parts[i]; gsub(/^[[:space:]]+|[[:space:]]+$/, "", c)
          if (c ~ /=/) { sub(/^[^"]*"/, "", c); sub(/".*$/, "", c) }
          if (c != "") print c
        }
      }
      if (depth <= 0 && closes > 0) inside=0
    }' "$1"
}

# Flow id and outcome of each line in the block that cmd/gen flows writes.
flow_scenarios() {
  awk '
    /^# BEGIN generated flow scenarios$/ { inside=1; next }
    /^# END generated flow scenarios$/ { inside=0 }
    inside && $2 == "-MonacoFlow" { print $3, $4 }' "$manifest"
}

# Manifest lines outside the generated flow block, as name<TAB>args.
hand_entries() {
  awk '
    /^# BEGIN generated flow scenarios$/ { skip=1 }
    /^# END generated flow scenarios$/ { skip=0 }
    { sub(/#.*/, "") }
    !skip && NF { name=$1; $1=""; sub(/^ +/, ""); print name "\t" $0 }' "$manifest"
}

check() {
  local missing=0 flag file flags count scenario id outcome harnesses name args first second hits extra
  harnesses="$(grep -rlE ':[[:space:]]*SampleHarnessEntry\b' "$app_src" --include='*.swift')"

  while read -r flag; do
    if ! in_manifest "$flag"; then
      echo "screens: $flag has no line in scripts/qa/sample-screens.txt" >&2
      missing=1
    fi
  done < <(grep -rhoE '"-(Monaco[A-Za-z]*(Sample|Gallery)[A-Za-z]*|[a-z][A-Za-z]*Harness)"' "$app_src" --include='*.swift' | tr -d '"' | sort -u)

  # $harnesses is split on purpose: app source paths have no spaces.
  # shellcheck disable=SC2086
  while read -r file; do
    flags="$(grep -oE '"-(Monaco[A-Za-z]*(Sample|Gallery)[A-Za-z]*|[a-z][A-Za-z]*Harness)"' "$file" | tr -d '"' | sort -u)"
    count="$(printf '%s\n' "$flags" | grep -c .)"
    if [[ "$count" != 1 ]]; then
      echo "screens: $(basename "$file") has a scenario enum but $count sample flags; cannot pair them" >&2
      missing=1
      continue
    fi
    while read -r scenario; do
      if ! in_manifest "$flags" "$scenario"; then
        echo "screens: $flags $scenario has no line in scripts/qa/sample-screens.txt" >&2
        missing=1
      fi
    done < <(enum_cases "$file")
  done < <(grep -lE 'enum [A-Za-z0-9_]*Sample[A-Za-z0-9_]*: String, CaseIterable' $harnesses)

  # Reverse: the flag must be read by a harness file, the scenario by that file or the mobile-core
  # preview sources it forwards to, and every further -Monaco flag by some app source.
  while IFS=$'\t' read -r name args; do
    # $args is split on purpose: it is a list of launch arguments with no spaces inside.
    # shellcheck disable=SC2086
    set -- $args
    first="$1"; second="${2:-}"
    # shellcheck disable=SC2086
    hits="$(grep -lF "\"$first\"" $harnesses)"
    if [[ -z "$hits" ]]; then
      echo "screens: $name launches $args, which no harness reads" >&2
      missing=1
      continue
    fi
    if [[ -n "$second" && "$second" != -* ]]; then
      # shellcheck disable=SC2086
      if ! grep -qE "\"$second\"|case[[:space:]].*\b$second\b" $hits &&
        ! grep -rqE "case[[:space:]]+$second\b" "$core_src" --include='*.swift'; then
        echo "screens: $name launches $args, which no harness reads" >&2
        missing=1
        continue
      fi
    fi
    for extra in "$@"; do
      [[ "$extra" == -Monaco* && "$extra" != "$first" ]] || continue
      if ! grep -rqF "\"$extra\"" "$app_src" --include='*.swift'; then
        echo "screens: $name launches $args, which no harness reads" >&2
        missing=1
        break
      fi
    done
  done < <(hand_entries)

  while read -r id outcome; do
    # shellcheck disable=SC2086
    if [[ -z "$harnesses" ]] || ! grep -qE "\bFlow${id}Scenario\.matching\(" $harnesses; then
      echo "screens: -MonacoFlow $id $outcome has no harness entry" >&2
      missing=1
    fi
  done < <(flow_scenarios)

  (( missing == 0 )) && echo "screens: manifest covers every sample harness ($(entries | wc -l | tr -d ' ') screens)"
  return "$missing"
}

capture() {
  local sim="$1" app="$2" dir="$3" bundle name args failed=0 shot=0 wait
  [[ -d "$app" ]] || { echo "screens: no app at $app" >&2; return 1; }
  bundle="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$app/Info.plist")" || return 1
  mkdir -p "$dir"
  xcrun simctl install "$sim" "$app" </dev/null || { echo "screens: install failed" >&2; return 1; }

  wait=$((settle * 3)) # the first launch after an install is the slow one
  while IFS=$'\t' read -r name args; do
    local start=$SECONDS took
    xcrun simctl terminate "$sim" "$bundle" </dev/null >/dev/null 2>&1
    # $args is split on purpose: it is a list of launch arguments with no spaces inside.
    # shellcheck disable=SC2086
    if ! xcrun simctl launch "$sim" "$bundle" $args </dev/null >/dev/null; then
      took=$((SECONDS - start))
      echo "screens: $name ${took}s did not launch"
      failed=$((failed + 1)); continue
    fi
    sleep "$wait"; wait="$settle"
    if xcrun simctl io "$sim" screenshot --type=png "$dir/$name.png" </dev/null >/dev/null 2>&1; then
      took=$((SECONDS - start))
      shot=$((shot + 1))
      echo "screens: $name ${took}s"
    else
      took=$((SECONDS - start))
      echo "screens: $name ${took}s could not be shot"
      failed=$((failed + 1))
    fi
  done < <(entries)
  xcrun simctl terminate "$sim" "$bundle" </dev/null >/dev/null 2>&1

  echo "screens: $shot shot, $failed failed, in $dir"
  check || failed=$((failed + 1))
  (( failed == 0 ))
}

case "${1:-}" in
  --check) check ;;
  -h|--help|"") sed -n '2,17p' "$0"; [[ -n "${1:-}" ]] ;;
  *)
    [[ $# -eq 3 ]] || { echo "usage: $0 --check | <sim udid> <Monaco.app> <out dir>" >&2; exit 2; }
    capture "$@"
    ;;
esac
