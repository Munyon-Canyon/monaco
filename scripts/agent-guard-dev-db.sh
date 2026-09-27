#!/usr/bin/env bash
# PreToolUse hook for Bash: refuse commands that can corrupt or silently weaken
# the local dev Postgres (monaco-postgres on 54322). Exit 2 blocks the command
# and shows stderr to the agent.
set -euo pipefail

if command -v jq >/dev/null 2>&1; then
  cmd="$(jq -r '.tool_input.command // empty')"
else
  cmd="$(python3 -c 'import json,sys; print(json.load(sys.stdin).get("tool_input",{}).get("command",""))')"
fi
[[ -z "$cmd" ]] && exit 0

lower="$(printf '%s' "$cmd" | tr '[:upper:]' '[:lower:]')"

block() {
  echo "blocked by scripts/agent-guard-dev-db.sh: $1" >&2
  echo "Benchmarks and durability experiments run in a throwaway container on another port, e.g." >&2
  echo "  docker run --rm -d --name bench-pg -p 54399:5432 -e POSTGRES_PASSWORD=bench postgres:16-alpine -c fsync=off" >&2
  echo "and are removed afterwards. See AGENTS.md (dev database rule)." >&2
  exit 2
}

if [[ "$lower" =~ alter[[:space:]]+system ]]; then
  block "ALTER SYSTEM persists in the data volume and survives restarts."
fi

if [[ "$lower" == *pg_resetwal* ]]; then
  block "pg_resetwal can leave the database silently inconsistent. Ask the user; the supported recovery is 'just reset db'."
fi

targets_dev=0
[[ "$lower" == *54322* || "$lower" == *monaco-postgres* || "$lower" == *"docker compose"* ]] && targets_dev=1
if (( targets_dev )) && [[ "$lower" =~ (fsync|synchronous_commit|full_page_writes)[[:space:]]*=[[:space:]]*\'?off ]]; then
  block "durability settings must stay on for the dev database."
fi

exit 0
