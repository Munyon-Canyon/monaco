#!/usr/bin/env bash
# Community, because from v0.38 the official build gates `atlas migrate lint` behind a login.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dest="$root/.bin"
version="$(cat "$root/apps/backend/.atlas-version")"

case "$(uname -s)-$(uname -m)" in
  Darwin-arm64) platform=darwin-arm64 ;;
  Darwin-x86_64) platform=darwin-amd64 ;;
  Linux-x86_64) platform=linux-amd64 ;;
  Linux-aarch64) platform=linux-arm64 ;;
  *) echo "error: no atlas build for $(uname -s)-$(uname -m)" >&2; exit 1 ;;
esac

url="https://release.ariga.io/atlas/atlas-community-${platform}-${version}"
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
curl -sSfL --retry 3 --retry-delay 2 --retry-all-errors -o "$tmp" "$url"
want="$(curl -sSfL --retry 3 --retry-delay 2 --retry-all-errors "${url}.sha256" | awk '{print $1}')"
if command -v sha256sum >/dev/null 2>&1; then
  have="$(sha256sum "$tmp" | awk '{print $1}')"
else
  have="$(shasum -a 256 "$tmp" | awk '{print $1}')"
fi
if [[ "$have" != "$want" ]]; then
  echo "error: atlas ${version} checksum mismatch (got ${have}, want ${want})" >&2
  exit 1
fi
mkdir -p "$dest"
install -m 0755 "$tmp" "$dest/atlas"
"$dest/atlas" version | head -1
