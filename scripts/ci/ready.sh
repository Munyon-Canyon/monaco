#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../.."
if [[ -n "$(git status --porcelain --untracked-files=all)" ]]; then
  echo "error: ready checks the committed tree, and this one has uncommitted changes. Commit or stash them and rerun." >&2
  git status --short >&2
  exit 1
fi

fresh() {
  local what=$1
  if [[ -n "$(git status --porcelain --untracked-files=all)" ]]; then
    echo "error: $what is stale. Run it, commit the result and push:" >&2
    git status --short >&2
    git diff --stat >&2
    exit 1
  fi
}

# sqlc expands * when it generates, so two branches that merge cleanly can leave a stale column list.
if star=$(grep -rnEi --include='*.sql' 'select[[:space:]]+\*|returning[[:space:]]+\*|\.\*' apps/backend/queries); then
  echo "error: apps/backend/queries must list columns, not use *. Fix these file:line matches:" >&2
  echo "$star" >&2
  exit 1
fi

cd apps/backend
toolchain="$(awk '$1 == "toolchain" {print $2}' go.mod)"
# A newer local Go overrides go.mod's toolchain line and writes different generated files
# (go1.27 names json.RawMessage jsontext.Value in docs/reference/events.md), so every step runs on CI's toolchain.
export GOTOOLCHAIN="${toolchain:?apps/backend/go.mod has no toolchain line}"
go vet ./...
go mod tidy -diff
go generate ./...
fresh "go generate ./..."
go run ./cmd/monacoctl flows check --structure-only
go run ./cmd/monacoctl migrate order
echo "ready: on go.mod's toolchain, vet, go.mod tidy, everything go generate writes (sqlc, atlas.sum and the reference docs among it), the flow files and migration order are all current"
