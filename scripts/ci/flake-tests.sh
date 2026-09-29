#!/usr/bin/env bash
set -euo pipefail

print_only=0
root=""
base=""
files=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --print) print_only=1; shift ;;
    --root) root="$2"; shift 2 ;;
    --base) base="$2"; shift 2 ;;
    --) shift; files+=("$@"); break ;;
    *) files+=("$1"); shift ;;
  esac
done

if [[ -z "$root" ]]; then
  root="$(cd "$(dirname "$0")/../.." && pwd)"
fi
cd "$root"

if [[ ${#files[@]} -eq 0 && -n "$base" ]]; then
  while IFS= read -r f; do
    [[ -n "$f" ]] && files+=("$f")
  done < <(git diff --name-only --diff-filter=d "$base"...HEAD -- 'apps/backend/**/*_test.go')
fi

if [[ ${#files[@]} -eq 0 ]]; then
  exit 0
fi

cmds="$(python3 - "${files[@]}" <<'PY'
import os, re, sys

groups = {}
order = []
for f in sys.argv[1:]:
    rel = f[2:] if f.startswith("./") else f
    if not rel.startswith("apps/backend/") or not rel.endswith("_test.go"):
        continue
    inside = rel[len("apps/backend/"):]
    pkg = os.path.dirname(inside) or "."
    spec = "." if pkg == "." else "./" + pkg
    path = f if os.path.isabs(f) else os.path.join(os.getcwd(), rel)
    text = open(path).read()
    names = re.findall(r"(?m)^func\s+(?:\([^)]*\)\s+)?(Test\w+)\s*\(", text)
    if spec not in groups:
        groups[spec] = []
        order.append(spec)
    groups[spec].extend(names)
for spec in order:
    names = groups[spec]
    if names:
        alt = "|".join(dict.fromkeys(names))
        print(f"go test -tags faultpoints -short -count=20 -cpu=1,2 -run '^({alt})$' {spec}")
    else:
        print(f"go test -tags faultpoints -short -count=20 -cpu=1,2 {spec}")
PY
)"

if [[ -z "$cmds" ]]; then
  exit 0
fi
if [[ "$print_only" -eq 1 ]]; then
  printf '%s\n' "$cmds"
  exit 0
fi
cd "$root/apps/backend"
while IFS= read -r cmd; do
  [[ -n "$cmd" ]] && eval "$cmd"
done <<<"$cmds"
