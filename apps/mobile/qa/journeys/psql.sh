#!/usr/bin/env bash
# psql for journey truth checks: the host psql when installed, else psql inside the Compose
# postgres container. Arguments go to psql, the query comes on stdin, DATABASE_URL names the
# database. Exits 127 when neither is available, so a caller never reports a missing client
# as a database that is down.
set -euo pipefail

container="${JOURNEY_PSQL_CONTAINER:-monaco-postgres}"

if command -v psql > /dev/null 2>&1; then
  exec psql "$DATABASE_URL" "$@"
fi

if command -v docker > /dev/null 2>&1 &&
  [[ "$(docker inspect -f '{{.State.Running}}' "$container" 2> /dev/null)" == "true" ]]; then
  database="${DATABASE_URL%%\?*}"
  database="${database##*/}"
  exec docker exec -i "$container" sh -c 'exec psql -U "$POSTGRES_USER" -d "$0" "$@"' "$database" "$@"
fi

echo "psql not found (install it, or start Compose postgres)" >&2
exit 127
