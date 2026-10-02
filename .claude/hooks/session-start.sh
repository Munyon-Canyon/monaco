#!/bin/bash
# Claude Code on the web: install jq (just test backend reads go test -json with it) and the
# pinned atlas, sqlc and govulncheck into .bin/ (install-dev.sh is interactive macOS), and start dockerd,
# because the container does not run it and processes started by the environment setup
# script don't survive into the session.
set -euo pipefail

if [[ "${CLAUDE_CODE_REMOTE:-}" != "true" ]]; then
  exit 0
fi

if ! command -v jq >/dev/null 2>&1; then
  apt-get install -y jq >/tmp/jq-install.log 2>&1 || echo "session-start: jq install failed; see /tmp/jq-install.log" >&2
fi

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
atlas_tag="$(cat "$root/apps/backend/.atlas-version")"
atlas_line="$("$root/.bin/atlas" version 2>/dev/null | head -1 || true)"
case "$atlas_line" in
  "atlas community version ${atlas_tag}"|"atlas version ${atlas_tag}") ;;
  *)
    if ! "$root/scripts/install-atlas.sh" >&2; then
      mkdir -p "$root/.bin"
      if ! GOBIN="$root/.bin" go install "ariga.io/atlas/cmd/atlas@${atlas_tag}"; then
        echo "session-start: atlas install failed" >&2
      fi
    fi
    ;;
esac
sqlc_want="v$(sed -n 's/^version=//p' "$root/scripts/install-sqlc.sh")"
if [[ "$("$root/.bin/sqlc" version 2>/dev/null)" != "$sqlc_want" ]]; then
  "$root/scripts/install-sqlc.sh" >&2 || echo "session-start: sqlc install failed" >&2
fi
govulncheck_want="$(sed -n 's/^version=//p' "$root/scripts/install-govulncheck.sh")"
if [[ ! -x "$root/.bin/govulncheck" || "$(cat "$root/.bin/govulncheck.version" 2>/dev/null)" != "$govulncheck_want" ]]; then
  "$root/scripts/install-govulncheck.sh" >&2 || echo "session-start: govulncheck install failed" >&2
fi

if docker info >/dev/null 2>&1; then
  exit 0
fi

if ! command -v dockerd >/dev/null 2>&1; then
  echo "session-start: dockerd not installed; skipping" >&2
  exit 0
fi

# A pid file cached from an earlier container (e.g. the setup script's dockerd) blocks startup.
if ! pgrep -x dockerd >/dev/null 2>&1; then
  rm -f /var/run/docker.pid
fi

nohup dockerd >/tmp/dockerd.log 2>&1 &
for _ in $(seq 1 30); do
  docker info >/dev/null 2>&1 && exit 0
  sleep 1
done

echo "session-start: dockerd failed to start; see /tmp/dockerd.log" >&2
tail -20 /tmp/dockerd.log >&2 || true
exit 1
