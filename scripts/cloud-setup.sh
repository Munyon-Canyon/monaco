#!/bin/bash
# Claude Code on the web: environment setup script. The environment config holds a pasted copy,
# because the repo may not be cloned yet when setup runs; this file is the source of truth, so
# paste it again after every edit. It installs tools and warms caches. Anything that must be
# running during the session (dockerd, atlas, sqlc) belongs in .claude/hooks/session-start.sh,
# because processes started here don't survive into the session.
set -euo pipefail

# Go: go.mod asks for 1.25.0; auto lets the preinstalled go fetch it from proxy.golang.org.
export GOTOOLCHAIN=auto

# just + dotenvx (usually preinstalled; installed only if missing)
command -v just >/dev/null || npm install -g rust-just
command -v dotenvx >/dev/null || npm install -g @dotenvx/dotenvx

# Swift (for `just test mobile` = host `swift test` in packages/mobile-core).
# Tarball + gpg check instead of swiftly: swift.org serves its keys gzip-encoded,
# which breaks swiftly's key import, so fetch them with --compressed.
SWIFT_VERSION=6.3.3
if ! command -v swift >/dev/null; then
  tmp=$(mktemp -d)
  base="https://download.swift.org/swift-${SWIFT_VERSION}-release/ubuntu2404/swift-${SWIFT_VERSION}-RELEASE/swift-${SWIFT_VERSION}-RELEASE-ubuntu24.04.tar.gz"
  curl -fsSL --compressed https://www.swift.org/keys/all-keys.asc -o "$tmp/keys.asc"
  gpg --batch --quiet --import "$tmp/keys.asc"
  curl -fsSL -o "$tmp/swift.tgz" "$base"
  curl -fsSL -o "$tmp/swift.tgz.sig" "$base.sig"
  gpg --batch --verify "$tmp/swift.tgz.sig" "$tmp/swift.tgz"
  mkdir -p /opt/swift
  tar xzf "$tmp/swift.tgz" -C /opt/swift --strip-components=1
  ln -sf /opt/swift/usr/bin/* /usr/local/bin/
  rm -rf "$tmp"
fi

go version; just --version; dotenvx --version; swift --version

# Docker daemon: start only if it isn't running, and fail loudly if it never comes up
if ! docker info >/dev/null 2>&1; then
  nohup dockerd >/tmp/dockerd.log 2>&1 &
  for _ in $(seq 1 30); do docker info >/dev/null 2>&1 && break; sleep 1; done
  docker info >/dev/null 2>&1 || { echo "error: dockerd failed to start"; tail -20 /tmp/dockerd.log; exit 1; }
fi

# Optional warm-up: best effort, never fails setup
docker pull postgres:16-alpine || true
if [[ -f /home/user/monaco/apps/backend/go.mod ]]; then
  (cd /home/user/monaco/apps/backend && go mod download) || true
fi
if [[ -f /home/user/monaco/packages/mobile-core/Package.swift ]]; then
  (cd /home/user/monaco/packages/mobile-core && swift package resolve) || true
fi
