#!/usr/bin/env bash
set -euo pipefail

if ! command -v docker >/dev/null 2>&1; then
  echo "error: docker is not on PATH. Install Docker Desktop and retry."
  exit 1
fi

# A hung daemon blocks `docker info` forever and macOS has no `timeout`, so poll the
# background process. MONACO_DOCKER_INFO_TIMEOUT is read only here; never export it.
limit="${MONACO_DOCKER_INFO_TIMEOUT:-15}"
docker info >/dev/null 2>&1 &
pid=$!
tenths=0
while kill -0 "$pid" 2>/dev/null; do
  if [ "$tenths" -ge $((limit * 10)) ]; then
    kill "$pid" 2>/dev/null || true
    echo "error: docker did not answer within ${limit}s; Docker Desktop is hung. Quit and reopen it (or: docker desktop restart), then retry."
    exit 1
  fi
  sleep 0.1
  tenths=$((tenths + 1))
done

if ! wait "$pid"; then
  echo "error: docker daemon is not running. Start Docker Desktop and retry."
  exit 1
fi
