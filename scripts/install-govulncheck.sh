#!/usr/bin/env bash
set -euo pipefail

dest="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/.bin"
version=v1.8.0

if [[ -x "$dest/govulncheck" && "$(cat "$dest/govulncheck.version" 2>/dev/null)" == "$version" ]]; then
  echo "govulncheck $version in $dest"
  exit 0
fi

mkdir -p "$dest"
GOBIN="$dest" GOFLAGS=-mod=mod go install "golang.org/x/vuln/cmd/govulncheck@${version}"
echo "$version" > "$dest/govulncheck.version"
echo "govulncheck $version in $dest"
