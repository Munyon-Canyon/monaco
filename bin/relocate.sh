#!/bin/bash
# Usage: bin/relocate.sh   (run once after unpacking this folder into <your clone>/.git/pstack/m7-rest)
# Rewrites the original machine's absolute paths in these files to your clone and your home.
set -euo pipefail
here=$(cd "$(dirname "$0")/.." && pwd)
clone=$(cd "$here/../../.." && pwd)
grep -rlI --exclude-dir=logs --exclude-dir=cache -e /Users/logno "$here" | while read -r f; do
  sed -i.bak -e "s#/private/tmp/claude-501/[^ ]*/scratchpad#$here/scratch#g" -e "s#/Users/logno/Developer/monaco#$clone#g" -e "s#/Users/logno#$HOME#g" "$f" && rm -f "$f.bak"
done
echo "relocated to $clone"
