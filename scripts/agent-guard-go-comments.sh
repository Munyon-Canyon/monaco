#!/usr/bin/env bash
# Claude Code shows stderr to the agent on exit 2. Cursor's postToolUse only
# passes additional_context from stdout JSON on exit 0 (cursor.com/docs/agent/hooks).
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd -P)"
common="$(git -C "$root" rev-parse --path-format=absolute --git-common-dir)"

read -r event path < <(python3 -c '
import json, sys
d = json.load(sys.stdin)
print(d.get("hook_event_name") or "-", d.get("tool_input", {}).get("file_path") or d.get("file_path") or "")
')

pass() {
  [[ "$event" == postToolUse ]] && echo '{}'
  exit 0
}

[[ -z "$path" ]] && pass

if [[ "$path" == /* ]]; then
  [[ -e "$path" ]] || pass
  dir="$(cd "$(dirname "$path")" && pwd -P)"
  path="$dir/$(basename "$path")"
  root="$(git -C "$dir" rev-parse --show-toplevel 2>/dev/null)" || pass
  [[ "$(git -C "$root" rev-parse --path-format=absolute --git-common-dir)" == "$common" ]] || pass
  path="${path#"$root"/}"
fi

[[ "$path" == apps/backend/*.go ]] || pass
[[ "$path" == */testdata/* ]] && pass
[[ -f "$root/$path" ]] || pass

cd "$root/apps/backend"
if out="$(go run ./cmd/monacoctl lint comments "${path#apps/backend/}" 2>&1)"; then
  pass
fi
message="$(
  while IFS= read -r line; do
    if [[ "$line" =~ ^(.+):([0-9]+):\ comment\ not\ allowed$ ]]; then
      printf 'apps/backend/%s:%s: comment not allowed: %s\n' "${BASH_REMATCH[1]}" "${BASH_REMATCH[2]}" \
        "$(sed -n "${BASH_REMATCH[2]}p" "${BASH_REMATCH[1]}" | sed 's/^[[:space:]]*//')"
    elif [[ ! "$line" =~ ^exit\ status\ [0-9]+$ ]]; then
      printf '%s\n' "$line"
    fi
  done <<<"$out"
  echo "No comments in Go. Use a better name, a type, a test, or an issue."
)"

if [[ "$event" == postToolUse ]]; then
  python3 -c 'import json, sys; print(json.dumps({"additional_context": sys.argv[1]}))' "$message"
  exit 0
fi
printf '%s\n' "$message" >&2
exit 2
