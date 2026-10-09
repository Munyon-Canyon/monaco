#!/usr/bin/env bash
# Run bin/monacoctl for a journey setup or truth script: the one place that knows which
# variables are the app's, not the backend's. monacoctl's config.Load rejects every MONACO_
# variable it does not know, and journey.py exports the slot's MONACO_API_BASE_URL and
# MONACO_QA_* overrides to the scripts it runs, so they are dropped here.
# Usage: scripts/qa/monacoctl.sh <monacoctl args...>
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root"

if [[ ! -x bin/monacoctl ]]; then
  echo "bin/monacoctl is missing: run just build backend first" >&2
  exit 1
fi

drop=()
while read -r name; do
  drop+=(-u "$name")
done < <(compgen -e | grep -E '^(MONACO_API_BASE_URL|MONACO_QA_.*)$' || true)

exec env ${drop[@]+"${drop[@]}"} scripts/with-dotenv-local.sh bin/monacoctl "$@"
