#!/usr/bin/env bash
set -euo pipefail

dest="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/.bin"
version=v1.8.0

mkdir -p "$dest"
GOBIN="$dest" GOFLAGS=-mod=mod go install "golang.org/x/vuln/cmd/govulncheck@${version}"
echo "$version" > "$dest/govulncheck.version"
echo "govulncheck $version in $dest"
