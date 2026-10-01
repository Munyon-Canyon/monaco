#!/usr/bin/env bash
set -euo pipefail

root="$(git rev-parse --show-toplevel)"
cd "$root"

files=()
while IFS= read -r file; do
  case "$file" in
    apps/mobile/*.swift) files+=("$file") ;;
    packages/mobile-core/*.swift) files+=("$file") ;;
  esac
done < <(git diff --cached --name-only --diff-filter=ACM)

if [[ ${#files[@]} -eq 0 ]]; then
  exit 0
fi

fail() {
  echo "pre-commit: comments or lint findings in staged Swift files. No comments in Swift. Use a better name, a type, a test, or an issue." >&2
  exit 1
}

swift format lint --strict "${files[@]}" || fail
scripts/swiftlint-ratchet.sh "${files[@]}" || fail
