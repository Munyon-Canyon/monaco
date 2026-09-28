#!/bin/bash
# Usage: once.sh <cmd...>   (run from inside a worktree)
# Runs <cmd> at most once per exact tree (committed + uncommitted + untracked) and command line.
# A pass is cached; a later call on the same tree prints the cached evidence and exits 0 without rerunning.
# Failures are never cached. ONCE_FORCE=1 reruns anyway.
set -euo pipefail
C=/Users/logno/Developer/monaco/.git/pstack/m7-rest/cache
top=$(git rev-parse --show-toplevel)
idx=$(mktemp); trap 'rm -f "$idx"' EXIT
cp "$(git rev-parse --git-dir)/index" "$idx" 2>/dev/null || true
tree=$(cd "$top" && GIT_INDEX_FILE="$idx" git add -A && GIT_INDEX_FILE="$idx" git write-tree)
key=$(printf '%s\0%s' "$tree" "$*" | shasum -a 256 | cut -c1-20)
if [[ -z "${ONCE_FORCE:-}" && -f "$C/$key.pass" ]]; then
  echo "once.sh: cached PASS for tree $tree: $*"
  cat "$C/$key.pass"
  exit 0
fi
log="$C/$key.log"
start=$(date +%s)
if "$@" >"$log" 2>&1; then
  printf 'tree=%s head=%s at=%s secs=%s log=%s\n' "$tree" "$(git rev-parse HEAD)" "$(date -u +%FT%TZ)" "$(( $(date +%s)-start ))" "$log" > "$C/$key.pass"
  tail -5 "$log"; echo "once.sh: PASS cached as $C/$key.pass"
else
  rc=$?; tail -30 "$log"; echo "once.sh: FAIL rc=$rc (not cached) log=$log"; exit $rc
fi
