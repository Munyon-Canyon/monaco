#!/usr/bin/env bash
set -euo pipefail

dest="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/.bin"
version=1.31.1

case "$(uname -s)-$(uname -m)" in
  Darwin-arm64) platform=darwin_arm64 sha=21602158c99eb1f2bae197a66abfb1941d1e9e50b23125bb193349c6b1acc71e ;;
  Darwin-x86_64) platform=darwin_amd64 sha=c5af76772e3785d21663a62697056b383f07629979b1bd25b93872e73dbd519b ;;
  Linux-x86_64) platform=linux_amd64 sha=497ae4fcdfa64c5b0c311ffe4c2bd991e43991e82e5367792ed78bc2dca27354 ;;
  Linux-aarch64) platform=linux_arm64 sha=b7cae247740d0c51a1e657479e5b2d21e6fef428f596682a01bc55bf4ab8a23d ;;
  *) echo "error: no sqlc build for $(uname -s)-$(uname -m)" >&2; exit 1 ;;
esac

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
curl -sSfL -o "$tmp/sqlc.tar.gz" \
  "https://github.com/sqlc-dev/sqlc/releases/download/v${version}/sqlc_${version}_${platform}.tar.gz"
if command -v sha256sum >/dev/null 2>&1; then
  have="$(sha256sum "$tmp/sqlc.tar.gz" | awk '{print $1}')"
else
  have="$(shasum -a 256 "$tmp/sqlc.tar.gz" | awk '{print $1}')"
fi
if [[ "$have" != "$sha" ]]; then
  echo "error: sqlc ${version} checksum mismatch (got ${have}, want ${sha})" >&2
  exit 1
fi
tar -xzf "$tmp/sqlc.tar.gz" -C "$tmp" sqlc
mkdir -p "$dest"
install -m 0755 "$tmp/sqlc" "$dest/sqlc"
echo "sqlc $("$dest/sqlc" version)"
