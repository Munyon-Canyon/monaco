#!/usr/bin/env bash
# The one docker run of mobile-integration.sh: swift test in the swift image with the checkout mounted at
# MOBILE_INTEGRATION_WORKDIR (default /w). CI sets it to the path the mobile-core job's container sees, because
# SwiftPM's build directory records absolute paths and only a matching mount reuses the restored cache.
# Usage: mobile-integration-swift.sh <image> <test filter>. Everything else comes from the caller's environment.
set -euo pipefail

root="$(cd "$(dirname "$0")/../.." && pwd)"
workdir="${MOBILE_INTEGRATION_WORKDIR:-/w}"
image="${1:?image}"
filter="${2:?filter}"

# The swift test flags must match swift_flags in scripts/mobile-core-test.sh: SwiftPM keys its build products on them,
# so other flags rebuild every project module instead of using the restored cache.
exec docker run --rm --network host \
  -v "$root:$workdir" -w "$workdir/packages/mobile-core" \
  -v "$MI_BINDIR/linux:/seed:ro" \
  -e MONACO_API_URL="$MI_API_URL" -e MONACO_DEV_TOKEN -e MONACO_DEV_USER \
  -e MONACO_SEED_BIN=/seed/monacoctl -e MONACO_SEED_DIR="$workdir/apps/backend" \
  -e DATABASE_URL="$MI_DATABASE_URL" -e NATS_URL="$MI_NATS_URL" \
  -e MONACO_DEV_TOKEN_KEY -e MONACO_FAKES_URL="$MI_FAKES_URL" \
  "$image" swift test --force-resolved-versions -Xswiftc -warnings-as-errors --enable-code-coverage --filter "$filter"
