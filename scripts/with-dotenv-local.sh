#!/usr/bin/env bash
# Run a command with repo-root .env.local decrypted via dotenvx.
# Key sources, in order: .env.keys in this checkout, .env.keys in the primary
# clone (so a worktree under .worktrees/ needs no copy), DOTENV_PRIVATE_KEY_LOCAL /
# DOTENV_PRIVATE_KEY in the environment, then Dotenvx Armor. A keys file counts
# only when it holds a DOTENV_PRIVATE_KEY_LOCAL line; it is passed by path, never
# read into this script.
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

has_local_key() {
  [[ -f "$1" ]] && grep -q '^DOTENV_PRIVATE_KEY_LOCAL=' "$1"
}

primary_root="$(dirname "$(git rev-parse --path-format=absolute --git-common-dir 2>/dev/null || echo "$repo_root/.git")")"
keys_args=()
if has_local_key "$repo_root/.env.keys"; then
  keys_args=(-fk "$repo_root/.env.keys")
  key_source=".env.keys in this checkout"
elif has_local_key "$primary_root/.env.keys"; then
  keys_args=(-fk "$primary_root/.env.keys")
  key_source=".env.keys in the primary clone at $primary_root"
else
  key_source="DOTENV_PRIVATE_KEY* env or Dotenvx Armor"
fi
echo "with-dotenv-local: key source: $key_source" >&2

exec dotenvx run -f .env.local ${keys_args[@]+"${keys_args[@]}"} -- "$@"
