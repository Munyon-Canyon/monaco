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
GOTOOLCHAIN="${toolchain:?apps/backend/go.mod has no toolchain line}" go vet ./...
go mod tidy -diff
go generate ./...
fresh "go generate ./..."
sqlc=../../.bin/sqlc
[[ -x "$sqlc" ]] || sqlc=sqlc
"$sqlc" diff
../../scripts/gen-docs.sh
fresh "scripts/gen-docs.sh"
go run ./cmd/monacoctl flows check --structure-only
go run ./cmd/monacoctl migrate order
echo "ready: vet on go.mod's toolchain, go.mod tidy, generated code, sqlc, reference docs, the flow files and migration order are all current"
