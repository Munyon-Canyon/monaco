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

fail() {
  if [[ "$event" == postToolUse ]]; then
    python3 -c 'import json, sys; print(json.dumps({"additional_context": sys.argv[1]}))' "$1"
    exit 0
  fi
  printf '%s\n' "$1" >&2
  exit 2
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

case "$path" in
  apps/mobile/*.swift) ;;
  packages/mobile-core/*.swift) ;;
  *) pass ;;
esac
case "$path" in
  */.build/*) pass ;;
esac
[[ -f "$root/$path" ]] || pass

cd "$root"
findings=""

read -r -d '' parse_format <<'PY' || true
import pathlib, re, sys
rel, blob = sys.argv[1], sys.argv[2]
source = pathlib.Path(rel).read_text(encoding="utf-8").splitlines()
pat = re.compile(r"^(.*?):(\d+):\d+: error: \[([^\]]+)\] (.*)$")
for line in blob.splitlines():
    match = pat.match(line)
    if not match:
        continue
    lineno = int(match.group(2))
    text = source[lineno - 1].strip() if 0 < lineno <= len(source) else ""
    print(f"{rel}:{lineno}: {match.group(3)}: {match.group(4)}: {text}")
PY

if ! fmt_out="$(swift format lint --strict "$path" 2>&1)"; then
  parsed="$(python3 -c "$parse_format" "$path" "$fmt_out")"
  findings="${parsed:-$fmt_out}"
fi

lint_bin=swiftlint
ready=0
if command -v "$lint_bin" >/dev/null 2>&1 && [[ "$("$lint_bin" version 2>/dev/null || true)" == "0.65.0" ]]; then
  ready=1
elif "$root/scripts/require-docker.sh" >/dev/null 2>&1; then
  ready=1
fi
if [[ $ready -eq 0 ]]; then
  note="swiftlint skipped: start Docker"
  if [[ -n "$findings" ]]; then
    note="${findings}"$'\n'"${note}"
  fi
  fail "$note"
fi

if ! lint_out="$(scripts/swiftlint-ratchet.sh "$path" 2>&1)"; then
  hits=""
  while IFS= read -r line; do
    case "$line" in
      "  "*) hits+="${line#"  "}"$'\n' ;;
    esac
  done <<<"$lint_out"
  hits="${hits%$'\n'}"
  if [[ -n "$findings" && -n "$hits" ]]; then
    findings="${findings}"$'\n'"${hits}"
  elif [[ -n "$hits" ]]; then
    findings="$hits"
  elif [[ -n "$lint_out" ]]; then
    findings="${findings:+$findings$'\n'}${lint_out}"
  fi
fi

[[ -z "$findings" ]] && pass
fail "$findings"
