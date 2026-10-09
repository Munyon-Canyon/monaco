#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
primary="$(dirname "$(git -C "$root" rev-parse --path-format=absolute --git-common-dir 2>/dev/null || echo "$root/.git")")"
dest="$primary/.bin"
version="$(sed -n 's/^version=//p' "$root/scripts/swiftlint-ratchet.sh")"

# SwiftLint publishes no checksum file, so this is pinned from the release asset.
# portable_swiftlint.zip is one universal binary for x86_64 and arm64.
pinned_version=0.65.0
pinned_sha=d6cb0aa7a2f5f1ef306fc9e37bcb54dc9a26facc8f7784ac0c3dd3eccf5c6ba6

if [[ "$(uname -s)" != Darwin ]]; then
  echo "error: portable_swiftlint.zip is a macOS build; ${version} runs from Docker on $(uname -s)" >&2
  exit 1
fi
if [[ "$version" != "$pinned_version" ]]; then
  echo "error: scripts/swiftlint-ratchet.sh pins swiftlint ${version}, but this script only knows the sha256 of ${pinned_version}. Update pinned_version and pinned_sha in scripts/install-swiftlint.sh." >&2
  exit 1
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$dest"

curl -sSfL -o "$tmp/portable_swiftlint.zip" \
  "https://github.com/realm/SwiftLint/releases/download/${version}/portable_swiftlint.zip"
have="$(shasum -a 256 "$tmp/portable_swiftlint.zip" | awk '{print $1}')"
if [[ "$have" != "$pinned_sha" ]]; then
  echo "error: swiftlint ${version} checksum mismatch (got ${have}, want ${pinned_sha})" >&2
  exit 1
fi
tar -xf "$tmp/portable_swiftlint.zip" -C "$tmp" swiftlint
install -m 0755 "$tmp/swiftlint" "$dest/swiftlint"

echo "swiftlint $("$dest/swiftlint" version)"
