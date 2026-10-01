#!/usr/bin/env bash
# Print a Build Timing Summary from an xcresult build log.
# xcodebuild test does not print one. Outermost build tasks are grouped by
# the first word of commandDetails, and their durations are summed.
set -euo pipefail

log="${BUILD_TIMING_LOG:-}"
if [[ -z "$log" ]]; then
  xcresult="${1:-}"
  if [[ -z "$xcresult" || ! -d "$xcresult" ]]; then
    exit 0
  fi
  log="$(mktemp)"
  trap 'rm -f "$log"' EXIT
  if ! xcrun xcresulttool get log --type build --path "$xcresult" --compact > "$log"; then
    exit 0
  fi
fi

python3 - "$log" << 'PY'
import json
import sys

with open(sys.argv[1]) as handle:
    data = json.load(handle)


def task_token(node):
    details = (node.get("commandInvocationDetails") or {}).get("commandDetails") or ""
    if not details:
        return ""
    line = details.strip().split("\n", 1)[0].replace("\\ ", " ")
    token = line.split(" ", 1)[0]
    if not token or not token[0].isupper() or not token.isalnum():
        return ""
    return token


counts = {}
durations = {}


def walk(node, parent):
    token = task_token(node)
    if token and token != parent:
        counts[token] = counts.get(token, 0) + 1
        durations[token] = durations.get(token, 0.0) + float(node.get("duration") or 0)
        child_parent = token
    else:
        child_parent = parent
    for child in node.get("subsections") or []:
        walk(child, child_parent)


if isinstance(data, dict):
    walk(data, "")

if not counts:
    sys.exit(0)

print("Build Timing Summary")
ranked = sorted(durations.items(), key=lambda item: (-item[1], item[0]))
for name, duration in ranked:
    count = counts[name]
    unit = "task" if count == 1 else "tasks"
    print(f"{name} ({count} {unit}) | {duration:.3f} seconds")
PY
