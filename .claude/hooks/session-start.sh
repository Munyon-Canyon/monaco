#!/bin/bash
# Claude Code on the web: the container does not run dockerd, and processes started by
# the environment setup script don't survive into the session. Start it here so
# `just test backend` / `just run backend` can bring up Compose Postgres.
set -euo pipefail

if [[ "${CLAUDE_CODE_REMOTE:-}" != "true" ]]; then
  exit 0
fi

if docker info >/dev/null 2>&1; then
  exit 0
fi

if ! command -v dockerd >/dev/null 2>&1; then
  echo "session-start: dockerd not installed; skipping" >&2
  exit 0
fi

# A pid file cached from an earlier container (e.g. the setup script's dockerd) blocks startup.
if ! pgrep -x dockerd >/dev/null 2>&1; then
  rm -f /var/run/docker.pid
fi

nohup dockerd >/tmp/dockerd.log 2>&1 &
for _ in $(seq 1 30); do
  docker info >/dev/null 2>&1 && exit 0
  sleep 1
done

echo "session-start: dockerd failed to start; see /tmp/dockerd.log" >&2
tail -20 /tmp/dockerd.log >&2 || true
exit 1
