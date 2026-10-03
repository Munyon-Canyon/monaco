#!/usr/bin/env bash
# Claude Code shows stderr to the agent on exit 2. Cursor's postToolUse only
# passes additional_context from stdout JSON on exit 0 (cursor.com/docs/agent/hooks).
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd -P)"

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
  path="$(cd "$(dirname "$path")" && pwd -P)/$(basename "$path")"
  [[ "$path" == "$root"/* ]] || pass
  path="${path#"$root"/}"
fi

case "$path" in
  *_test.go | apps/backend/coverage.exclude | apps/backend/mutants.allow | apps/backend/.golangci.yml) ;;
  */testdata/perf/baseline.json | */testdata/golden/*) ;;
  packages/mobile-core/Tests/*.swift | apps/mobile/MonacoTests/*.swift | apps/mobile/MonacoUITests/*.swift) ;;
  .swift-format | */.swift-format | .swiftlint.yml | */.swiftlint.yml | .swiftlint-baseline.tsv) ;;
  packages/mobile-core/legacy-baseline.tsv | packages/mobile-core/tsan-suppressions.txt) ;;
  packages/mobile-core/coverage-floor.txt | packages/mobile-core/Package.swift) ;;
  */RepoRulesAllowlist.txt | */AccessibilityAuditAllowlist.txt | apps/mobile/MonacoUITests/perf-budgets.tsv) ;;
  apps/mobile/Monaco.xcodeproj/project.pbxproj | apps/mobile/Config/*.xcconfig) ;;
  scripts/qa/sample-screens.txt) ;;
  .github/workflows/ci-mobile-core.yml|Justfile | apps/backend/cmd/monacoctl/agents/check.go | scripts/mobile-core-test.sh) ;;
  *) pass ;;
esac

cd "$root"
if out="$(python3 scripts/check-gate-changes.py --worktree "$path")"; then
  pass
fi
[[ -z "$out" ]] && pass
message="$(
  while IFS= read -r finding; do
    printf '%s. This weakens a regression gate. Revert it unless the user asked for it in this session; if they did, say so under Reviewer focus in the PR.\n' "$finding"
  done <<<"$out"
)"

if [[ "$event" == postToolUse ]]; then
  python3 -c 'import json, sys; print(json.dumps({"additional_context": sys.argv[1]}))' "$message"
  exit 0
fi
printf '%s\n' "$message" >&2
exit 2
