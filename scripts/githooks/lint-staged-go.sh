#!/usr/bin/env bash
set -euo pipefail

root="$(git rev-parse --show-toplevel)"
cd "$root"

pkgs=()
while IFS= read -r file; do
  [[ "$file" == */testdata/* ]] && continue
  pkg="./$(dirname "${file#apps/backend/}")"
  [[ " ${pkgs[*]-} " == *" $pkg "* ]] || pkgs+=("$pkg")
done < <(git diff --cached --name-only --diff-filter=ACM -- 'apps/backend/*.go')

if [[ ${#pkgs[@]} -eq 0 ]]; then
  exit 0
fi

if ! command -v golangci-lint >/dev/null 2>&1; then
  echo "error: golangci-lint is not on PATH. Run: just install" >&2
  exit 1
fi

cd apps/backend
if ! golangci-lint run --new-from-rev=HEAD "${pkgs[@]}"; then
  echo "pre-commit: golangci-lint found issues in staged apps/backend packages." >&2
  exit 1
fi
if ! go run ./internal/platform/lint/nogo/cmd/nogo "${pkgs[@]}"; then
  echo "pre-commit: nogo found bare go statements in staged apps/backend packages." >&2
  exit 1
fi
