#!/usr/bin/env bash
# Decrypt repo-root .env.local via dotenvx and materialize iOS Privy config.
# Writes gitignored Privy.local.xcconfig + Privy.local.Info.plist (merged into app at build;
# Debug uses Privy.local.Debug.Info.plist, the same keys plus the ATS local-networking
# exception that Release must not carry).
# Also writes gitignored Environment.local.xcconfig with the staging / production API base
# URLs (optional, non-secret) that Config/Monaco.xcconfig resolves MONACO_API_BASE_URL from.
# `placeholder` writes a compile-only config instead (CI and sample-data QA; no secret needed).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="$ROOT/apps/mobile/Config/Privy.local.xcconfig"
PLIST_OUT="$ROOT/apps/mobile/Config/Privy.local.Info.plist"
DEBUG_PLIST_OUT="$ROOT/apps/mobile/Config/Privy.local.Debug.Info.plist"
ENVIRONMENT_OUT="$ROOT/apps/mobile/Config/Environment.local.xcconfig"
ENV_FILE="$ROOT/.env.local"
# Reset with the guard. A second source in the same shell must look up the
# key file again; leaving dotenv_keys_resolved set would skip that lookup
# and run dotenvx get with an empty keys_args.
keys_args=()
dotenv_keys_resolved=""

# One dotenvx get, with -fk when a keys file was resolved. A failing get
# becomes an empty value, matching the previous `|| true`.
dotenvx_get() {
  local value=""
  if ! value="$(cd "$ROOT" && dotenvx get "$1" -f .env.local ${keys_args[@]+"${keys_args[@]}"} 2>/dev/null)"; then
    value=""
  fi
  printf '%s' "$value"
}

load_privy_env() {
  if [[ ! -f "$ENV_FILE" ]]; then
    echo "error: $ENV_FILE missing — set PRIVY_APP_ID and PRIVY_APP_CLIENT_ID with dotenvx." >&2
    exit 1
  fi
  if ! command -v dotenvx >/dev/null 2>&1; then
    echo "error: dotenvx not on PATH. Install: https://dotenvx.com/docs/install" >&2
    exit 1
  fi
  # Same key lookup as with-dotenv-local.sh. Run git from ROOT inside the
  # substitution so sourcing this script does not change the caller's directory.
  # A keys file counts only when it holds DOTENV_PRIVATE_KEY_LOCAL; otherwise
  # leave -fk unset so an exported DOTENV_PRIVATE_KEY_LOCAL / DOTENV_PRIVATE_KEY
  # still works for cloud agents.
  if [[ -z "${dotenv_keys_resolved:-}" ]]; then
    dotenv_keys_resolved=1
    has_local_key() {
      [[ -f "$1" ]] && grep -q '^DOTENV_PRIVATE_KEY_LOCAL=' "$1"
    }

    primary_root="$(dirname "$(cd "$ROOT" && { git rev-parse --path-format=absolute --git-common-dir 2>/dev/null || echo "$ROOT/.git"; })")"
    keys_args=()
    if has_local_key "$ROOT/.env.keys"; then
      keys_args=(-fk "$ROOT/.env.keys")
      key_source=".env.keys in this checkout"
    elif has_local_key "$primary_root/.env.keys"; then
      keys_args=(-fk "$primary_root/.env.keys")
      key_source=".env.keys in the primary clone at $primary_root"
    else
      key_source="DOTENV_PRIVATE_KEY* env or Dotenvx Armor"
    fi
    echo "ensure-ios-privy-config: key source: $key_source" >&2
  fi
  # Always read from .env.local. dotenvx get/run both honor existing shell exports,
  # so clear stale Privy keys before fetching decrypted values.
  unset PRIVY_APP_ID PRIVY_APP_CLIENT_ID PRIVY_AUTH_ID \
    PRIVY_SMS_LOGIN_ENABLED PRIVY_EMAIL_LOGIN_ENABLED PRIVY_AUTHORIZATION_KEY_ID \
    POSTHOG_API_KEY

  PRIVY_APP_ID="$(dotenvx_get PRIVY_APP_ID)"
  PRIVY_APP_CLIENT_ID="$(dotenvx_get PRIVY_APP_CLIENT_ID)"
  PRIVY_AUTH_ID="$(dotenvx_get PRIVY_AUTH_ID)"
  PRIVY_SMS_LOGIN_ENABLED="$(dotenvx_get PRIVY_SMS_LOGIN_ENABLED)"
  PRIVY_EMAIL_LOGIN_ENABLED="$(dotenvx_get PRIVY_EMAIL_LOGIN_ENABLED)"
  PRIVY_AUTHORIZATION_KEY_ID="$(dotenvx_get PRIVY_AUTHORIZATION_KEY_ID)"
  POSTHOG_API_KEY="$(dotenvx_get POSTHOG_API_KEY)"
  PRIVY_SMS_LOGIN_ENABLED="${PRIVY_SMS_LOGIN_ENABLED:-true}"
  PRIVY_EMAIL_LOGIN_ENABLED="${PRIVY_EMAIL_LOGIN_ENABLED:-true}"
}

resolve_ios_client_id() {
  local client_id="${PRIVY_APP_CLIENT_ID:-}"
  if [[ -z "$client_id" && -n "${PRIVY_AUTH_ID:-}" && "${PRIVY_AUTH_ID}" == client-* ]]; then
    client_id="$PRIVY_AUTH_ID"
  fi
  if [[ -z "${PRIVY_APP_ID:-}" || -z "$client_id" || "$client_id" != client-* ]]; then
    echo "error: PRIVY_APP_ID and PRIVY_APP_CLIENT_ID (client-…) must be set in .env.local" >&2
    exit 1
  fi
  printf '%s' "$client_id"
}

# ATS stays strict everywhere; Debug alone may reach a dev backend over plain http on
# the local network (this Mac's LAN IP / .local name from a device build).
debug_ats_plist_fragment() {
  cat <<'EOF'
	<key>NSAppTransportSecurity</key>
	<dict>
		<key>NSAllowsLocalNetworking</key>
		<true/>
	</dict>
EOF
}

generate_info_plist() {
  local client_id="$1"
  write_info_plist "$PLIST_OUT" "$client_id" ""
  write_info_plist "$DEBUG_PLIST_OUT" "$client_id" "$(debug_ats_plist_fragment)"
}

write_info_plist() {
  local plist_out="$1" client_id="$2" extra_keys="$3"
  mkdir -p "$(dirname "$plist_out")"
  cat >"$plist_out" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>PRIVY_APP_ID</key>
	<string>${PRIVY_APP_ID}</string>
	<key>PRIVY_APP_CLIENT_ID</key>
	<string>${client_id}</string>
	<key>PRIVY_SMS_LOGIN_ENABLED</key>
	<string>${PRIVY_SMS_LOGIN_ENABLED:-true}</string>
	<key>PRIVY_EMAIL_LOGIN_ENABLED</key>
	<string>${PRIVY_EMAIL_LOGIN_ENABLED:-true}</string>
	<key>PRIVY_AUTHORIZATION_KEY_ID</key>
	<string>${PRIVY_AUTHORIZATION_KEY_ID:-}</string>
	<key>MONACO_ENVIRONMENT</key>
	<string>\$(MONACO_ENVIRONMENT)</string>
	<key>MONACO_API_BASE_URL</key>
	<string>\$(MONACO_API_BASE_URL)</string>
	<key>MonacoAPSEnvironment</key>
	<string>\$(MONACO_APS_ENVIRONMENT)</string>
	<key>POSTHOG_API_KEY</key>
	<string>${POSTHOG_API_KEY:-}</string>
	<key>CFBundleURLTypes</key>
	<array>
		<dict>
			<key>CFBundleURLName</key>
			<string>com.monaco.app</string>
			<key>CFBundleURLSchemes</key>
			<array>
				<string>monaco</string>
			</array>
		</dict>
	</array>
${extra_keys}
</dict>
</plist>
EOF
}

# Remote API base URLs are optional, but a value that is set must be https: the app
# refuses anything else in Release, so fail here instead of at launch on a tester's phone.
remote_api_base_url() {
  local key="$1" value
  value="$(dotenvx_get "$key")"
  if [[ -n "$value" && "$value" != https://?* ]]; then
    echo "error: $key must be an https:// URL (got: $value)" >&2
    exit 1
  fi
  printf '%s' "$value"
}

# xcconfig reads `//` as the start of a comment; `/$()/` expands back to `//`.
xcconfig_escape_url() {
  # shellcheck disable=SC2016 # single quotes keep $() literal for xcconfig
  local slashes='//' escaped='/$()/'
  printf '%s' "${1//$slashes/$escaped}"
}

generate_environment_xcconfig() {
  local staging production
  staging="$(remote_api_base_url MONACO_STAGING_API_BASE_URL)" || return 1
  production="$(remote_api_base_url MONACO_PRODUCTION_API_BASE_URL)" || return 1

  mkdir -p "$(dirname "$ENVIRONMENT_OUT")"
  cat >"$ENVIRONMENT_OUT" <<EOF
// Generated by scripts/ensure-ios-privy-config.sh — do not commit.
MONACO_STAGING_API_BASE_URL = $(xcconfig_escape_url "$staging")
MONACO_PRODUCTION_API_BASE_URL = $(xcconfig_escape_url "$production")
EOF
}

generate_xcconfig() {
  load_privy_env
  local client_id
  client_id="$(resolve_ios_client_id)"

  mkdir -p "$(dirname "$OUT")"
  cat >"$OUT" <<EOF
// Generated by scripts/ensure-ios-privy-config.sh — do not commit.
PRIVY_APP_ID = ${PRIVY_APP_ID}
PRIVY_APP_CLIENT_ID = ${client_id}
PRIVY_SMS_LOGIN_ENABLED = ${PRIVY_SMS_LOGIN_ENABLED:-true}
PRIVY_EMAIL_LOGIN_ENABLED = ${PRIVY_EMAIL_LOGIN_ENABLED:-true}
PRIVY_AUTHORIZATION_KEY_ID = ${PRIVY_AUTHORIZATION_KEY_ID:-}
EOF
  generate_info_plist "$client_id"
  generate_environment_xcconfig
}

PLACEHOLDER_MARKER="// PLACEHOLDER: compile-only, no Privy app."

# Compile-only config for CI and sample-data QA: no app or client ID, so the app builds and
# the Debug sample harnesses run, while the real login path shows that sign-in is not
# configured. Needs no .env.local and no secret. Refuses to replace a real generated config,
# so running it on a developer Mac never silently breaks sign-in; `generate` replaces a
# placeholder.
generate_placeholder() {
  if [[ -f "$OUT" ]] && ! grep -qF "$PLACEHOLDER_MARKER" "$OUT"; then
    echo "error: $OUT holds a real Privy config; not replacing it with a placeholder." >&2
    echo "       Delete it first (\`generate\` recreates it from .env.local)." >&2
    exit 1
  fi
  PRIVY_APP_ID=""
  PRIVY_SMS_LOGIN_ENABLED=true
  PRIVY_EMAIL_LOGIN_ENABLED=true
  PRIVY_AUTHORIZATION_KEY_ID=""
  POSTHOG_API_KEY=""
  mkdir -p "$(dirname "$OUT")"
  cat >"$OUT" <<EOF
// Generated by scripts/ensure-ios-privy-config.sh placeholder — do not commit.
$PLACEHOLDER_MARKER
// Sign-in cannot work with this build; only the Debug sample-data screens do.
PRIVY_APP_ID =
PRIVY_APP_CLIENT_ID =
PRIVY_SMS_LOGIN_ENABLED = true
PRIVY_EMAIL_LOGIN_ENABLED = true
PRIVY_AUTHORIZATION_KEY_ID =
EOF
  generate_info_plist ""
  mkdir -p "$(dirname "$ENVIRONMENT_OUT")"
  cat >"$ENVIRONMENT_OUT" <<EOF
// Generated by scripts/ensure-ios-privy-config.sh placeholder — do not commit.
MONACO_STAGING_API_BASE_URL =
MONACO_PRODUCTION_API_BASE_URL =
EOF
  echo "wrote placeholder Privy config (compile-only, no app ID)"
}

export_launch_env() {
  load_privy_env
  local client_id
  client_id="$(resolve_ios_client_id)"
  export PRIVY_APP_ID
  export PRIVY_APP_CLIENT_ID="$client_id"
  export PRIVY_SMS_LOGIN_ENABLED="${PRIVY_SMS_LOGIN_ENABLED:-true}"
  export PRIVY_EMAIL_LOGIN_ENABLED="${PRIVY_EMAIL_LOGIN_ENABLED:-true}"
  export SIMCTL_CHILD_PRIVY_APP_ID="$PRIVY_APP_ID"
  export SIMCTL_CHILD_PRIVY_APP_CLIENT_ID="$client_id"
  export SIMCTL_CHILD_PRIVY_SMS_LOGIN_ENABLED="${PRIVY_SMS_LOGIN_ENABLED:-true}"
  export SIMCTL_CHILD_PRIVY_EMAIL_LOGIN_ENABLED="${PRIVY_EMAIL_LOGIN_ENABLED:-true}"
  export PRIVY_AUTHORIZATION_KEY_ID="${PRIVY_AUTHORIZATION_KEY_ID:-}"
  export SIMCTL_CHILD_PRIVY_AUTHORIZATION_KEY_ID="${PRIVY_AUTHORIZATION_KEY_ID:-}"
  # Debug-only override: point a simulator at staging or a tunnel without rebuilding,
  # e.g. MONACO_API_BASE_URL=https://… just run mobile. Unset → the build's own URL.
  if [[ -n "${MONACO_API_BASE_URL:-}" ]]; then
    export SIMCTL_CHILD_MONACO_API_BASE_URL="$MONACO_API_BASE_URL"
  fi
  if [[ -n "${MONACO_ENVIRONMENT:-}" ]]; then
    export SIMCTL_CHILD_MONACO_ENVIRONMENT="$MONACO_ENVIRONMENT"
  fi
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  case "${1:-generate}" in
    generate)
      generate_xcconfig
      ;;
    --launch-env)
      generate_xcconfig
      export_launch_env
      ;;
    placeholder)
      generate_placeholder
      ;;
    *)
      echo "usage: ensure-ios-privy-config.sh [generate|--launch-env|placeholder]" >&2
      exit 1
      ;;
  esac
fi
