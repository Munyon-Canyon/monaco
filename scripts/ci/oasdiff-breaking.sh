#!/usr/bin/env bash
# Fail when <head> breaks clients of <base>. Adding an ErrorCode value is allowed: the code list
# grows with the errs table, and clients fall back on message for codes they do not know.
# docs/architecture/backend-platform.md#thin-client
set -euo pipefail

base="$1"
head="$2"
levels="$(dirname "$0")/oasdiff-levels.txt"
oasdiff breaking --fail-on ERR --severity-levels "$levels" "$base" "$head"
