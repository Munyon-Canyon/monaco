#!/usr/bin/env bash
# Compare two `go test -bench` outputs with benchstat and fail on any metric that got more than
# 10% worse with p < 0.05. docs/architecture/backend-platform.md#ci-gates
#
#   scripts/ci/bench-regressions.sh <old.txt> <new.txt>
set -euo pipefail

old="${1:?usage: bench-regressions.sh <old.txt> <new.txt>}"
new="${2:?usage: bench-regressions.sh <old.txt> <new.txt>}"

benchstat -format csv "$old" "$new" 2>/dev/null | awk -F, '
  /^pkg: / { pkg = substr($0, 6); next }
  $6 == "vs base" { unit = $2; next }
  $1 == "" || $1 == "geomean" || $6 !~ /^\+[0-9.]+%$/ { next }
  {
    delta = substr($6, 2, length($6) - 2) + 0
    split($7, p, /[= ]/)
    if (delta > 10 && p[2] + 0 < 0.05) {
      printf "%s %s %s: %s (%s)\n", pkg, $1, unit, $6, $7
      bad = 1
    }
  }
  END { exit bad }'
