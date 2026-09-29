#!/usr/bin/env bash
# Installs the repo's pstack model roles for Claude Code.
# Writes ~/.claude/pstack-models.md from docs/agents/pstack-models.md and adds its
# @-include to ~/.claude/CLAUDE.md. A second run changes nothing.
# Usage:
#   scripts/setup-agent-env.sh            # write what is missing
#   scripts/setup-agent-env.sh --dry-run  # print what it would write
#   scripts/setup-agent-env.sh --force    # also replace a sheet that differs
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
src="$repo_root/docs/agents/pstack-models.md"
dir="$HOME/.claude"
sheet="$dir/pstack-models.md"
claude_md="$dir/CLAUDE.md"
include="@~/.claude/pstack-models.md"

dry=0
force=0
for arg in "$@"; do
  case "$arg" in
    --dry-run) dry=1 ;;
    --force) force=1 ;;
    *)
      printf 'usage: scripts/setup-agent-env.sh [--dry-run] [--force]\n' >&2
      exit 2
      ;;
  esac
done

say() { printf '%s\n' "$*"; }

write_sheet() {
  if [[ "$dry" -eq 1 ]]; then
    say "would write $sheet"
    return
  fi
  mkdir -p "$dir"
  cp "$src" "$sheet"
  say "wrote $sheet"
}

if [[ ! -f "$sheet" ]]; then
  write_sheet
elif git diff --no-index --quiet -- "$sheet" "$src"; then
  say "$sheet is current"
else
  git --no-pager diff --no-index -- "$sheet" "$src" || true
  if [[ "$force" -eq 0 ]]; then
    say "error: $sheet differs from docs/agents/pstack-models.md (diff above). Rerun with --force to replace it." >&2
    exit 1
  fi
  write_sheet
fi

if [[ -f "$claude_md" ]] && grep -qxF "$include" "$claude_md"; then
  say "$claude_md already includes the sheet"
elif [[ "$dry" -eq 1 ]]; then
  say "would add $include to $claude_md"
else
  mkdir -p "$dir"
  printf '\n%s\n' "$include" >>"$claude_md"
  say "added $include to $claude_md"
fi

say "plugins: open the repo in Claude Code and trust the folder. .claude/settings.json then registers the team marketplaces and enables the plugins."
say "plugins: superpowers installs from its own repo, so run once: claude plugin install superpowers@claude-plugins-official --scope project"
