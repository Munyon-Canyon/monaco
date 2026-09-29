#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../apps/backend"
exec go run ./cmd/monacoctl agents forecast "$@"
