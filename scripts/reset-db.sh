#!/usr/bin/env bash
# Wipe the local Docker Compose Postgres volume and start it empty (localhost only).
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

./scripts/require-docker.sh
source ./scripts/assert-local-database-url.sh

echo "resetting local Postgres (docker compose down -v)..."
docker compose down -v

echo "starting Postgres..."
docker compose up -d --wait

./scripts/verify-local-db.sh

echo "local Postgres reset complete."
echo "  DATABASE_URL=${DATABASE_URL}"
