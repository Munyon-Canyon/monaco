#!/usr/bin/env bash
# Prove oasdiff-breaking.sh passes a new error code and fails a removed path.
set -euo pipefail

spec="${1:-apps/backend/api/openapi.yaml}"
check="$(dirname "$0")/oasdiff-breaking.sh"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

awk '{ print } /^    ErrorCode:$/,/^      enum:$/ { if ($0 ~ /enum:$/) print "        - zzz_new_code" }' \
  "$spec" > "$work/new-code.yaml"
grep -q 'zzz_new_code' "$work/new-code.yaml"
sed 's#^  /healthz:#  /healthz-renamed:#' "$spec" > "$work/removed-path.yaml"
grep -q 'healthz-renamed' "$work/removed-path.yaml"

"$check" "$spec" "$work/new-code.yaml" >/dev/null || { echo "a new ErrorCode value failed oasdiff-breaking.sh" >&2; exit 1; }
if "$check" "$spec" "$work/removed-path.yaml" >/dev/null; then
  echo "a removed path passed oasdiff-breaking.sh" >&2
  exit 1
fi
echo "oasdiff-breaking.sh: new error code passes, removed path fails"
