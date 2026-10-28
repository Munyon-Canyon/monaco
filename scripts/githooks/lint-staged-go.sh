#!/usr/bin/env bash
set -euo pipefail

root="$(git rev-parse --show-toplevel)"
cd "$root"

pkgs=()
files=()
staged=()
while IFS= read -r file; do
  [[ "$file" == */testdata/* ]] && continue
  files+=("${file#apps/backend/}")
  staged+=("$file")
  pkg="./$(dirname "${file#apps/backend/}")"
  [[ " ${pkgs[*]-} " == *" $pkg "* ]] || pkgs+=("$pkg")
done < <(git diff --cached --name-only --diff-filter=ACM -- 'apps/backend/*.go')

if [[ ${#pkgs[@]} -eq 0 ]]; then
  exit 0
fi

lint=golangci-lint
if ! command -v golangci-lint >/dev/null 2>&1; then
  lint="$(go env GOPATH)/bin/golangci-lint"
fi
if ! command -v "$lint" >/dev/null 2>&1; then
  echo "error: golangci-lint is not on PATH. Run: just install" >&2
  exit 1
fi

hash_before=()
unstaged=()
for file in "${staged[@]}"; do
  hash_before+=("$(git hash-object -- "$file")")
  if [[ -n "$(git diff --name-only -- "$file")" ]]; then unstaged+=(1); else unstaged+=(0); fi
done

(cd apps/backend && "$lint" fmt "${files[@]}") || true

partial=0
for i in "${!staged[@]}"; do
  file="${staged[$i]}"
  [[ "$(git hash-object -- "$file")" == "${hash_before[$i]}" ]] && continue
  if [[ "${unstaged[$i]}" -eq 1 ]]; then
    echo "pre-commit: $file has unstaged changes, formatted but not re-staged; stage it and commit again" >&2
    partial=1
  else
    git add -- "$file"
    echo "pre-commit: formatted $file"
  fi
done
[[ "$partial" -eq 0 ]] || exit 1

cd apps/backend
if ! "$lint" run --allow-parallel-runners --new-from-rev=HEAD "${pkgs[@]}"; then
  echo "pre-commit: golangci-lint found issues in staged apps/backend packages." >&2
  exit 1
fi
if ! go run ./internal/platform/lint/nogo/cmd/nogo "${pkgs[@]}"; then
  echo "pre-commit: nogo found bare go statements in staged apps/backend packages." >&2
  exit 1
fi
if ! go run ./cmd/monacoctl lint comments "${files[@]}"; then
  echo "pre-commit: comments found in staged apps/backend Go files. No comments in Go. Use a better name, a type, a test, or an issue." >&2
  exit 1
fi
