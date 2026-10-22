#!/usr/bin/env bash
# Refuses a staged line that holds a simulator UDID under .cursor/, docs/ (except docs/legacy/) and AGENTS.md files.
# Usage: check-staged-udid.sh [path ...]
#        git diff --cached --name-only --diff-filter=ACM | check-staged-udid.sh
set -euo pipefail

pattern='[0-9A-F]{8}-([0-9A-F]{4}-){3}[0-9A-F]{12}'

in_scope() {
  case "$1" in
    docs/legacy/*) return 1 ;;
    .cursor/* | docs/* | AGENTS.md | */AGENTS.md) return 0 ;;
  esac
  return 1
}

check_one() {
  local file="$1"
  in_scope "$file" || return 0
  local line hits=""
  while IFS= read -r line; do
    if [[ "$line" == +* && "$line" != "+++"* ]] && grep -qE "$pattern" <<<"$line"; then
      hits+="$line"$'\n'
    fi
  done < <(git diff --cached --unified=0 --diff-filter=ACM -- "$file")
  if [[ -n "$hits" ]]; then
    echo "error: $file adds a simulator UDID; resolve it at run time with scripts/gold-sim-udid.sh or scripts/resolve-ios-sim.sh" >&2
    printf '%s\n' "$hits" >&2
    return 1
  fi
  return 0
}

fail=0
if [[ $# -gt 0 ]]; then
  for file in "$@"; do
    check_one "$file" || fail=1
  done
else
  while IFS= read -r file || [[ -n "${file:-}" ]]; do
    [[ -z "$file" ]] && continue
    check_one "$file" || fail=1
  done
fi

exit "$fail"
