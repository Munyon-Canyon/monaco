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
# When that download is blocked, shims run the same tools in swift:6.3-noble.
SWIFT_VERSION=6.3.3
swift_shim=0
if grep -Fq 'mirror.gcr.io/library/swift:6.3-noble' /usr/local/bin/swift 2>/dev/null; then
  swift_shim=1
fi
install_swift() {
  tmp=$(mktemp -d) || return 1
  trap 'rm -rf "$tmp"' RETURN
  base="https://download.swift.org/swift-${SWIFT_VERSION}-release/ubuntu2404/swift-${SWIFT_VERSION}-RELEASE/swift-${SWIFT_VERSION}-RELEASE-ubuntu24.04.tar.gz"
  curl -fsSL --compressed https://www.swift.org/keys/all-keys.asc -o "$tmp/keys.asc" || return 1
  gpg --batch --quiet --import "$tmp/keys.asc" || return 1
  curl -fsSL -o "$tmp/swift.tgz" "$base" || return 1
  curl -fsSL -o "$tmp/swift.tgz.sig" "$base.sig" || return 1
  gpg --batch --verify "$tmp/swift.tgz.sig" "$tmp/swift.tgz" || return 1
  mkdir -p /opt/swift || return 1
  tar xzf "$tmp/swift.tgz" -C /opt/swift --strip-components=1 || return 1
  ln -sf /opt/swift/usr/bin/* /usr/local/bin/ || return 1
}
write_swift_shims() {
  mkdir -p /usr/local/bin
  local tool
  for tool in swift llvm-cov; do
    cat >"/usr/local/bin/${tool}" <<EOF
#!/bin/bash
set -euo pipefail
root="\$(git rev-parse --show-toplevel)"
exec docker run --rm -v "\$root:\$root" -w "\$PWD" mirror.gcr.io/library/swift:6.3-noble ${tool} "\$@"
EOF
    chmod 0755 "/usr/local/bin/${tool}"
  done
}
if ! command -v swift >/dev/null && [[ "$swift_shim" -eq 0 ]]; then
  if ! install_swift; then
    write_swift_shims
    swift_shim=1
  fi
fi

go version; just --version; dotenvx --version
if [[ "$swift_shim" -eq 0 ]]; then
  swift --version
else
  echo "swift: docker shims installed at /usr/local/bin/swift and /usr/local/bin/llvm-cov" >&2
fi

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
