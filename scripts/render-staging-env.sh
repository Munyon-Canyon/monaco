#!/usr/bin/env bash
# Push every key in the encrypted .env.staging, plus the NATS .creds file, into Render's monaco-staging env group.
# Usage, from the repo root: scripts/render-staging-env.sh <path-to-nats.creds>
# Auth: RENDER_API_KEY, else the token `render login` saved in ~/.render/cli.yaml.
# Each key is a PUT, so a rerun converges. Keys removed from .env.staging stay in the group until deleted in the dashboard.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
api="https://api.render.com/v1"
group="monaco-staging"

if [[ $# -ne 1 || ! -f "$1" ]]; then
  echo "usage: scripts/render-staging-env.sh <path-to-nats.creds>" >&2
  exit 1
fi
creds="$1"

token="${RENDER_API_KEY:-}"
if [[ -z "$token" ]]; then
  token="$(awk '/^ *key:/ {print $2; exit}' "$HOME/.render/cli.yaml" 2>/dev/null || true)"
fi
if [[ -z "$token" ]]; then
  echo "error: set RENDER_API_KEY or run render login" >&2
  exit 1
fi

render_api() {
  curl -sSf -H "Authorization: Bearer $token" -H "Accept: application/json" -H "Content-Type: application/json" "$@"
}

group_id="$(render_api "$api/env-groups?name=$group&limit=20" | jq -r --arg g "$group" '.[] | (.envGroup // .) | select(.name == $g) | .id' | head -1)"
if [[ -z "$group_id" ]]; then
  echo "error: no Render env group named $group; create the render.yaml Blueprint first" >&2
  exit 1
fi

env_json="$(cd "$repo_root" && dotenvx get -f .env.staging)"
count=0
while IFS= read -r key; do
  jq -n --arg v "$(jq -r --arg k "$key" '.[$k]' <<<"$env_json")" '{value: $v}' |
    render_api -X PUT --data @- "$api/env-groups/$group_id/env-vars/$key" >/dev/null
  count=$((count + 1))
done < <(jq -r 'keys[] | select(startswith("DOTENV_") | not)' <<<"$env_json")

jq -n --rawfile c "$creds" '{content: $c}' |
  render_api -X PUT --data @- "$api/env-groups/$group_id/secret-files/nats.creds" >/dev/null

echo "$group ($group_id): $count env vars and secret file nats.creds updated"
