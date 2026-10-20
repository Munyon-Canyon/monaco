#!/usr/bin/env bash
# The watchlist lives on the device, so the run writes nothing. The shared browse check verifies the seeded catalogue and removes it.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
exec apps/mobile/qa/journeys/stocks/browse.truth.sh
