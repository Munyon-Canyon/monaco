#!/usr/bin/env bash
set -euo pipefail

dest="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/.bin"
version=v0.6.0

mkdir -p "$dest"
GOBIN="$dest" GOFLAGS=-mod=mod go install "github.com/go-gremlins/gremlins/cmd/gremlins@${version}"
echo "$version" > "$dest/gremlins.version"
echo "gremlins $version in $dest"
