#!/usr/bin/env bash
# Connect this machine to the staging NATS account on Synadia Cloud.
# Decrypts NATS_STAGING_CREDS from .env.staging to ~/.config/monaco/staging.creds, saves the
# nats CLI context monaco-staging, and selects it for the nats-channel Claude Code plugin.
# Needs the .env.staging private key (shared like the .env.local key) and the nats CLI. Reruns converge.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
creds="$HOME/.config/monaco/staging.creds"
context="monaco-staging"

mkdir -p "$(dirname "$creds")"
(umask 077 && dotenvx get NATS_STAGING_CREDS -f "$repo_root/.env.staging" >"$creds")
grep -q 'BEGIN NATS USER JWT' "$creds" || {
  echo "error: could not decrypt NATS_STAGING_CREDS from .env.staging; get the staging private key" >&2
  exit 1
}

nats context save "$context" --server tls://connect.ngs.global --creds "$creds" --description "Monaco staging" >/dev/null

channel_config="$HOME/.claude/channels/nats/config.json"
mkdir -p "$(dirname "$channel_config")"
existing="$(cat "$channel_config" 2>/dev/null || echo '{}')"
jq --arg c "$context" '.context = $c' <<<"$existing" >"$channel_config"

nats --context "$context" stream ls --names
echo "connected: nats --context $context; run /reload-plugins in Claude Code to use it from nats-channel"
