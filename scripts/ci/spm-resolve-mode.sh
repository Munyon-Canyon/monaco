#!/usr/bin/env bash
# Print "exact" when SourcePackages was restored from the Package.resolved key
# this checkout hashes to, and workspace-state.json is there. Print "inexact"
# for a cold tree, a missing state file, or a prefix restore of some other key.
# SPM_HIT is actions/cache cache-hit ("true" only on the primary key).
# SPM_FETCH is the warm-artifact line ("artifact run <id>", "nothing", or empty).
# STATE_FILE is the workspace-state.json path.
set -euo pipefail

hit="${SPM_HIT:-}"
fetch="${SPM_FETCH:-}"
state="${STATE_FILE:-}"

if [[ -f "$state" ]]; then
  if [[ "$hit" == "true" || "$fetch" == "artifact run "* ]]; then
    echo exact
    exit 0
  fi
fi
echo inexact
