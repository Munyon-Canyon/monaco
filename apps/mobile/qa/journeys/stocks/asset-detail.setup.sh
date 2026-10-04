#!/usr/bin/env bash
set -euo pipefail
exec apps/mobile/qa/journeys/stocks/browse.setup.sh "${1:?usage: asset-detail.setup.sh <scenario>}"
