#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

./scripts/require-docker.sh
source ./scripts/assert-local-database-url.sh

case "${1:-}" in
  --all)
    echo "resetting local Postgres and NATS (docker compose down -v)..."
    docker compose --profile test down -v
    ;;
  "")
    echo "resetting local Postgres (NATS data kept)..."
    docker compose rm --stop --force postgres
    docker volume ls -q \
      --filter "label=com.docker.compose.project=${COMPOSE_PROJECT_NAME:-monaco}" \
      --filter "label=com.docker.compose.volume=monaco_pg_data" |
      xargs -r docker volume rm
    ;;
  *)
    echo "usage: reset-db.sh [--all]" >&2
    exit 2
    ;;
esac

echo "starting Postgres and NATS..."
docker compose up -d --wait

./scripts/verify-local-db.sh

echo "local reset complete."
echo "  DATABASE_URL=${DATABASE_URL}"
