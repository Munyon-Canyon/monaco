#!/usr/bin/env bash
# Run a command with repo-root .env.local decrypted via dotenvx.
# Decryption key: dotenvx reads .env.keys in the clone root, or falls back to
# DOTENV_PRIVATE_KEY_LOCAL / DOTENV_PRIVATE_KEY exported in the shell
# environment (e.g. a cloud environment variable), so no .env.keys file is needed.
# Usage: scripts/with-dotenv-local.sh <command> [args...]
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

if [[ ! -f .env.local ]]; then
  echo "error: .env.local missing — copy .env.example to .env.local, or place a teammate encrypted .env.local plus .env.keys in the clone root (or export DOTENV_PRIVATE_KEY)." >&2
  exit 1
fi

if ! command -v dotenvx >/dev/null 2>&1; then
  echo "error: dotenvx not on PATH. Install: https://dotenvx.com/docs/install" >&2
  exit 1
fi

if [[ $# -lt 1 ]]; then
  echo "usage: scripts/with-dotenv-local.sh <command> [args...]" >&2
  exit 1
fi

exec dotenvx run -f .env.local -- "$@"
