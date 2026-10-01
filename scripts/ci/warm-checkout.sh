#!/usr/bin/env bash
# Check out CHECKOUT_SHA when this run is a warm dispatch. The caller invokes
# this only for workflow_dispatch. The SHA must be an ancestor of
# origin/$FEATURE_BRANCH or origin/main.
set -euo pipefail

sha="${CHECKOUT_SHA:?}"
feature="${FEATURE_BRANCH:?}"

# actions/checkout is depth 1, so an ancestor check needs the rest of history.
git fetch --unshallow origin || true
git fetch origin "$feature" main
if ! git cat-file -e "${sha}^{commit}" 2>/dev/null; then
  git fetch origin "$sha"
fi

if git merge-base --is-ancestor "$sha" "origin/${feature}" \
  || git merge-base --is-ancestor "$sha" origin/main; then
  git checkout --detach "$sha"
  echo "checked out ${sha}"
  exit 0
fi

echo "refusing ${sha}: not an ancestor of origin/${feature} or origin/main" >&2
exit 1
