#!/usr/bin/env bash
# Print yes when an xcodebuild -list log, or the DerivedData it was given,
# shows Swift package resolution. Print no otherwise.
set -euo pipefail

log="${1:-}"
derived="${2:-}"

if [[ -n "$log" && -f "$log" ]]; then
  if grep -E -q -i 'Resolve Package Graph|Resolved source packages|Fetching from|Creating working copy|Downloading ' "$log"; then
    echo yes
    exit 0
  fi
fi

if [[ -n "$derived" && -d "$derived/SourcePackages" ]]; then
  echo yes
  exit 0
fi

echo no
