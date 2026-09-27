#!/usr/bin/env bash
# Fail when a workflow names a runner label outside GitHub's standard free set. Larger runners
# are billed even on public repos. docs/architecture/ci.md#staying-on-the-free-tier
set -euo pipefail

dir="${1:-.github/workflows}"
allowed=" ubuntu-latest ubuntu-24.04 ubuntu-24.04-arm macos-15 "
bad=0

while IFS=: read -r file _ value; do
  label="$(sed -E "s/^[[:space:]]*runs-on:[[:space:]]*//; s/[[:space:]]*(#.*)?$//; s/^['\"]//; s/['\"]$//" <<<"$value")"
  if [[ "$allowed" != *" $label "* ]]; then
    echo "$file: runs-on '$label' is not a standard runner (allowed:$allowed)" >&2
    bad=1
  fi
done < <(grep -HnE '^[[:space:]]*runs-on:' "$dir"/*.yml "$dir"/*.yaml 2>/dev/null || true)

exit "$bad"
