#!/usr/bin/env bash
set -euo pipefail

dest="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/.bin"
version=1.5.1
# v1.5.1 is an annotated tag. This is the commit it resolves to.
tag_commit=9243c70e78fb9d797e03a7b9d73926dee1022857

# xcsift publishes no checksum file, so these are pinned from the release assets.
# v1.5.1 has no Darwin-x86_64 asset; that platform builds the pinned tag below.
case "$(uname -s)-$(uname -m)" in
  Darwin-arm64) platform=macos-arm64 sha=5ea6c5befad58f881896e4fd1479b3727db0f5d43829e77666b54b0508efa989 ;;
  Linux-x86_64) platform=linux-x64 sha=5e903ce8c5bfa569465cf5659e836686574ee15e3608a5689c3727ab9994306e ;;
  Darwin-x86_64) platform=source ;;
  *) echo "error: no xcsift build for $(uname -s)-$(uname -m)" >&2; exit 1 ;;
esac

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$dest"

if [[ "$platform" == "source" ]]; then
  git clone --depth 1 --branch "v${version}" https://github.com/ldomaradzki/xcsift.git "$tmp/xcsift"
  got="$(git -C "$tmp/xcsift" rev-parse HEAD)"
  if [[ "$got" != "$tag_commit" ]]; then
    echo "error: xcsift v${version} resolved to ${got}, want ${tag_commit}" >&2
    exit 1
  fi
  # A release build prints VERSION_PLACEHOLDER until this replacement, which is what the
  # published archives do in their release workflow. --version must equal the version= line.
  sed -i '' "s/VERSION_PLACEHOLDER/${version}/g" "$tmp/xcsift/Sources/xcsift/main.swift"
  swift build -c release --disable-sandbox --package-path "$tmp/xcsift"
  product=$(find "$tmp/xcsift/.build" -type f -name xcsift -path '*/release/*' -print -quit)
  if [[ ! -x "$product" ]]; then
    echo "error: xcsift release binary missing under ${tmp}/xcsift/.build" >&2
    exit 1
  fi
  install -m 0755 "$product" "$dest/xcsift"
else
  curl -sSfL -o "$tmp/xcsift.tar.gz" \
    "https://github.com/ldomaradzki/xcsift/releases/download/v${version}/xcsift-v${version}-${platform}.tar.gz"
  if command -v sha256sum >/dev/null 2>&1; then
    have="$(sha256sum "$tmp/xcsift.tar.gz" | awk '{print $1}')"
  else
    have="$(shasum -a 256 "$tmp/xcsift.tar.gz" | awk '{print $1}')"
  fi
  if [[ "$have" != "$sha" ]]; then
    echo "error: xcsift ${version} checksum mismatch (got ${have}, want ${sha})" >&2
    exit 1
  fi
  tar -xzf "$tmp/xcsift.tar.gz" -C "$tmp" xcsift
  install -m 0755 "$tmp/xcsift" "$dest/xcsift"
fi

echo "xcsift $("$dest/xcsift" --version)"
