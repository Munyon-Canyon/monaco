#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
workflow="$root/.github/workflows/ci-jobs.yml"

files=()
if [[ $# -gt 0 ]]; then
  files=("$@")
else
  while IFS= read -r line; do
    [[ -n "$line" ]] && files+=("$line")
  done
fi

if [[ ${#files[@]} -eq 0 ]]; then
  exit 0
fi

python3 - "$workflow" "${files[@]}" <<'PY'
import re, sys

workflow, *files = sys.argv[1:]
text = open(workflow).read()
marker = "filters: |"
start = text.find(marker)
if start < 0:
    sys.exit("ci-jobs.yml has no filters block")
block = []
for line in text[start + len(marker):].splitlines()[1:]:
    if line.strip() == "" or not line.startswith("            "):
        break
    block.append(line)
filters = {}
order = []
name = None
for line in block:
    m = re.match(r"^            ([a-z0-9-]+):\s*$", line)
    if m:
        name = m.group(1)
        filters[name] = []
        order.append(name)
        continue
    m = re.match(r"""^              - '([^']+)'\s*$""", line)
    if m and name:
        filters[name].append(m.group(1))

def glob_match(pattern, path):
    i = 0
    out = ["^"]
    while i < len(pattern):
        c = pattern[i]
        if c == "*":
            if i + 1 < len(pattern) and pattern[i + 1] == "*":
                if i + 2 < len(pattern) and pattern[i + 2] == "/":
                    out.append("(?:.*/)?")
                    i += 3
                    continue
                out.append(".*")
                i += 2
                continue
            out.append("[^/]*")
        elif c == "?":
            out.append("[^/]")
        elif c in ".+()|^${}[]\\":
            out.append("\\" + c)
        else:
            out.append(c)
        i += 1
    out.append("$")
    return re.match("".join(out), path) is not None

gates = {
    "backend": "just test backend",
    "backend-go": "just test backend",
    "mobile-core": "cd packages/mobile-core && swift test",
    "ios": "just build mobile",
    "scripts": "cd scripts && go test ./...",
    "workflows": 'docker run --rm -v "$PWD:/repo" -w /repo rhysd/actionlint:1.7.12',
    "web": "cd apps/web && npm test",
}
seen = set()
for name in order:
    cmd = gates.get(name)
    if not cmd or cmd in seen:
        continue
    if any(glob_match(pat, f) for f in files for pat in filters.get(name, [])):
        seen.add(cmd)
        print(cmd)
PY


