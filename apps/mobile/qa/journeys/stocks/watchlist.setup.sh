#!/usr/bin/env bash
# journey.py runs this before each scenario with the scenario id. The watchlist journey reads the same seeded catalogue as stocks/browse.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
exec apps/mobile/qa/journeys/stocks/browse.setup.sh "${1:?usage: watchlist.setup.sh <scenario>}"
