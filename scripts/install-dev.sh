#!/usr/bin/env bash
# Interactive macOS setup for a clone of this repo.
# Asks before each install. Never installs without a yes.
# Usage:
#   ./scripts/install-dev.sh          # prompt for missing tools
#   ./scripts/install-dev.sh --check  # report only, exit 1 if required tools missing
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

check_only=0
if [[ "${1:-}" == "--check" ]]; then
  check_only=1
fi

missing_required=0
asked_any=0

say() { printf '%s\n' "$*"; }
err() { printf 'error: %s\n' "$*" >&2; }

ask_yes() {
  local prompt="$1"
  local reply
  if [[ "$check_only" -eq 1 ]]; then
    return 1
  fi
  asked_any=1
  if [[ ! -t 0 ]]; then
    err "stdin is not a TTY. rerun without --check in a terminal, or install the tool yourself."
    return 1
  fi
  read -r -p "${prompt} [y/N] " reply
  [[ "$reply" == "y" || "$reply" == "Y" || "$reply" == "yes" ]]
}

have() { command -v "$1" >/dev/null 2>&1; }

dev_tools=(
  awk basename bc benchstat brew caffeinate cat chmod cp curl cut date dirname
  docker dotenvx du ffmpeg ffprobe find gh git go golangci-lint grep gt head id
  install jq just kill ln ls lsof magick mkdir mktemp npm oasdiff open pgrep
  pkill python3 rm sed seq sha256sum shasum simslim sleep sort swift sysctl tail
  tar tee tr uname uuidgen wc xcode-select xcodebuild xcrun
)
: "${dev_tools[@]}"

run_brew() {
  local pkg="$1"
  if ! have brew; then
    err "Homebrew is not on PATH. install from https://brew.sh then rerun."
    return 1
  fi
  brew install "$pkg"
}

# --- required ---

if ! xcode-select -p >/dev/null 2>&1; then
  missing_required=1
  say "Xcode Command Line Tools are missing."
  if ask_yes "Install Xcode Command Line Tools now?"; then
    xcode-select --install || true
    say "Finish the installer GUI, then rerun ./scripts/install-dev.sh"
  fi
fi

if ! have xcodebuild; then
  missing_required=1
  say "xcodebuild is missing. Install Xcode from the App Store, open it once, then rerun."
fi

if ! have docker; then
  missing_required=1
  say "Docker is missing (needed for local Postgres)."
  if ask_yes "Open Docker Desktop install page?"; then
    open "https://docs.docker.com/desktop/setup/install/mac-install/" || true
  fi
fi

if ! have go; then
  missing_required=1
  say "Go is missing (1.23+)."
  if ask_yes "Install Go with Homebrew (go)?"; then
    run_brew go || missing_required=1
  fi
fi

golangci_want="$(cat apps/backend/.golangci-lint-version)"
golangci_have="$(golangci-lint version --short 2>/dev/null || true)"
if [[ "v${golangci_have#v}" != "$golangci_want" ]]; then
  missing_required=1
  say "golangci-lint ${golangci_want} is missing (found: ${golangci_have:-none}). It lints apps/backend in CI and pre-commit."
  if have go && ask_yes "Install golangci-lint ${golangci_want} with go install?"; then
    go install "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${golangci_want}" || missing_required=1
    say "installed into $(go env GOPATH)/bin. Put it ahead of other golangci-lint copies on PATH."
  fi
fi

if ! have jq; then
  missing_required=1
  say "jq is missing. just test backend and CI use it to read go test -json."
  if [[ "$(uname -s)" == "Linux" ]]; then
    if ask_yes "Install jq with apt-get?"; then
      if [[ "$(id -u)" -eq 0 ]]; then apt-get install -y jq || missing_required=1; else sudo apt-get install -y jq || missing_required=1; fi
    fi
  elif ask_yes "Install jq with Homebrew (jq)?"; then
    run_brew jq || missing_required=1
  fi
fi

atlas_want="$(cat apps/backend/.atlas-version)"
atlas_have="$(.bin/atlas version 2>/dev/null | head -1 || true)"
if [[ "$atlas_have" != "atlas community version ${atlas_want}" ]]; then
  missing_required=1
  say "atlas community ${atlas_want} is missing from .bin/atlas (found: ${atlas_have:-none}). monacoctl migrate runs that exact binary."
  if ask_yes "Install atlas community ${atlas_want} into .bin/?"; then
    ./scripts/install-atlas.sh || missing_required=1
  fi
fi

gremlins_want="$(sed -n 's/^version=//p' scripts/install-gremlins.sh)"
gremlins_have="$(cat .bin/gremlins.version 2>/dev/null || true)"
if [[ ! -x .bin/gremlins || "$gremlins_have" != "$gremlins_want" ]]; then
  missing_required=1
  say "gremlins ${gremlins_want} is missing from .bin/gremlins (found: ${gremlins_have:-none}). just test mutation runs that exact binary."
  if ask_yes "Install gremlins ${gremlins_want} into .bin/?"; then
    ./scripts/install-gremlins.sh || missing_required=1
  fi
fi

govulncheck_want="$(sed -n 's/^version=//p' scripts/install-govulncheck.sh)"
govulncheck_have="$(cat .bin/govulncheck.version 2>/dev/null || true)"
if [[ ! -x .bin/govulncheck || "$govulncheck_have" != "$govulncheck_want" ]]; then
  missing_required=1
  say "govulncheck ${govulncheck_want} is missing from .bin/govulncheck (found: ${govulncheck_have:-none}). just test vuln runs that exact binary."
  if ask_yes "Install govulncheck ${govulncheck_want} into .bin/?"; then
    ./scripts/install-govulncheck.sh || missing_required=1
  fi
fi

sqlc_want="v$(sed -n 's/^version=//p' scripts/install-sqlc.sh)"
sqlc_have="$(.bin/sqlc version 2>/dev/null || true)"
if [[ "$sqlc_have" != "$sqlc_want" ]]; then
  missing_required=1
  say "sqlc ${sqlc_want} is missing from .bin/sqlc (found: ${sqlc_have:-none}). It generates apps/backend query code from queries/."
  if ask_yes "Install sqlc ${sqlc_want} into .bin/?"; then
    ./scripts/install-sqlc.sh || missing_required=1
  fi
fi

if ! have just; then
  missing_required=1
  say "just is missing (https://github.com/casey/just)."
  if ask_yes "Install just with Homebrew (just)?"; then
    run_brew just || missing_required=1
  fi
fi

if ! have dotenvx; then
  missing_required=1
  say "dotenvx is missing (decrypts .env.local)."
  if ask_yes "Install dotenvx with Homebrew (dotenvx/brew/dotenvx)?"; then
    if have brew; then
      brew tap dotenvx/brew
      brew trust dotenvx/brew || true
      brew install dotenvx || missing_required=1
    else
      err "Homebrew is not on PATH."
    fi
  fi
fi

if ! have gt; then
  missing_required=1
  say "Graphite (gt) is missing. PRs are opened as stacks with gt submit (https://graphite.dev)."
  if ask_yes "Install Graphite with Homebrew (withgraphite/tap/graphite)?"; then
    if have brew; then
      brew install withgraphite/tap/graphite || missing_required=1
      say "Then run: gt auth --token <token from https://app.graphite.com/activate> && gt init --trunk main"
    else
      err "Homebrew is not on PATH."
    fi
  fi
fi

# --- optional SimSlim ---
if ! have simslim; then
  say "SimSlim is optional. Stock Xcode Simulator works. just run mobile falls back if slim is missing."
  if ask_yes "Install SimSlim with Homebrew (mobai-app/tap/simslim)?"; then
    if have brew; then
      brew tap mobai-app/tap
      brew install simslim || true
    fi
  fi
fi

# --- optional agent Phantom wallet ---
if [[ "$check_only" -eq 0 ]]; then
  say "A Phantom MCP wallet is optional. Use it only if a coding agent must send mainnet USDC during QA."
  if ask_yes "Print Phantom MCP setup steps (no install)?"; then
    say ""
    say "1. In Cursor, install the phantom-connect plugin (marketplace), or add @phantom/mcp-server to mcp.json."
    say "2. Docs: https://docs.phantom.com/phantom-mcp-server/setup"
    say "3. Do not put PHANTOM_APP_ID in .env.local. Agent wallet is separate from Privy product wallets."
    say "4. Fund the agent address on Solana mainnet only when you need live deposit QA. Refund leftover USDC when done."
    say ""
  fi
fi

# --- git hook ---
if [[ -d .git && ! -f .git/hooks/pre-commit ]]; then
  if ask_yes "Install the pre-commit hook (blocks plaintext .env commits and new golangci-lint findings)?"; then
    chmod +x scripts/githooks/pre-commit scripts/githooks/check-staged-env.sh
    ln -sfn ../../scripts/githooks/pre-commit .git/hooks/pre-commit
    say "installed .git/hooks/pre-commit"
  fi
fi

# --- env files ---
if [[ ! -f .env.local ]]; then
  missing_required=1
  err ".env.local is missing."
  say "Put an encrypted .env.local next to .env.keys (from a teammate), or copy .env.example to .env.local and fill Privy + relayer values with dotenvx set."
  if [[ "$check_only" -eq 0 ]] && ask_yes "Copy .env.example to .env.local now (you still must set secrets)?"; then
    cp .env.example .env.local
    say "wrote .env.local from .env.example"
  fi
fi

if [[ ! -f .env.keys ]] && [[ -z "${DOTENV_PRIVATE_KEY:-}" ]]; then
  say "No .env.keys and DOTENV_PRIVATE_KEY is unset. dotenvx may still use macOS Keychain if you created the key on this Mac."
  say "If a teammate encrypted .env.local, place their .env.keys in the repo root (gitignored)."
fi

if have dotenvx && [[ -f .env.local ]]; then
  if ! dotenvx get PRIVY_APP_ID -f .env.local >/dev/null 2>&1; then
    missing_required=1
    err "dotenvx cannot read PRIVY_APP_ID from .env.local. check .env.keys and that the file is encrypted for that key."
  else
    client="$(dotenvx get PRIVY_APP_CLIENT_ID -f .env.local 2>/dev/null || true)"
    auth="$(dotenvx get PRIVY_AUTH_ID -f .env.local 2>/dev/null || true)"
    if [[ -z "$client" && -z "$auth" ]]; then
      missing_required=1
      err "PRIVY_APP_CLIENT_ID (or PRIVY_AUTH_ID) is empty. iOS will show Privy not configured."
    else
      say "Privy app id is readable from .env.local."
    fi
  fi
fi

if [[ -d .git/hooks ]] && [[ ! -f .git/hooks/pre-commit ]] && [[ "$check_only" -eq 1 ]]; then
  say "note: git pre-commit hook is not installed. run ./scripts/install-dev.sh to copy it."
fi

say ""
if [[ "$missing_required" -ne 0 ]]; then
  err "required setup is incomplete. install missing tools, then place .env.local and .env.keys, then rerun --check."
  exit 1
fi

if [[ "$check_only" -eq 1 ]]; then
  say "required tools and Privy env look ready. next: just run"
  exit 0
fi

if [[ "$asked_any" -eq 0 ]]; then
  say "required tools already on PATH."
fi
say "next: just run   (Postgres + API + iOS). just run mobile injects Privy even without SimSlim."
exit 0
