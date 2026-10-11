#!/usr/bin/env bash
# Push the Monaco Business Grafana folder, Infinity data source and dashboards in
# ops/grafana/ to Grafana Cloud. Idempotent: every object is upserted by uid.
#
# Grafana credentials (GRAFANA_URL, GRAFANA_SERVICE_ACCOUNT_TOKEN) come from the
# decrypted .env.local via scripts/with-dotenv-local.sh. The staging API base URL
# comes from MONACO_API_BASE_URL, else PUBLIC_API_BASE_URL in .env.staging,
# decrypted with the DOTENV_PRIVATE_KEY_STAGING in this checkout's or the primary
# clone's .env.keys.
#
# The data source bearer token is a Grafana secure field. Without --set-token the
# sync keeps the secret Grafana already holds. With --set-token it reads the
# token from stdin (never argv) and replaces it:
#   bin/monacoctl admin token create --name grafana | scripts/grafana-sync.sh --set-token
#
# Usage: scripts/grafana-sync.sh [--set-token]
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

set_token=0
case "${1:-}" in
  "") ;;
  --set-token) set_token=1 ;;
  *) echo "usage: scripts/grafana-sync.sh [--set-token]" >&2; exit 64 ;;
esac

if [[ -z "${GRAFANA_URL:-}" || -z "${GRAFANA_SERVICE_ACCOUNT_TOKEN:-}" ]]; then
  exec "$repo_root/scripts/with-dotenv-local.sh" "$0" "$@"
fi
command -v jq >/dev/null || { echo "error: jq not on PATH" >&2; exit 1; }
grafana_url="${GRAFANA_URL%/}"

api_base="${MONACO_API_BASE_URL:-}"
# .env.local sets a local PUBLIC_API_BASE_URL, and dotenvx never overrides a variable that is already set.
unset PUBLIC_API_BASE_URL
if [[ -z "$api_base" ]]; then
  primary_root="$(dirname "$(git rev-parse --path-format=absolute --git-common-dir)")"
  keys_file=""
  for candidate in "$repo_root/.env.keys" "$primary_root/.env.keys"; do
    if [[ -f "$candidate" ]] && grep -q '^DOTENV_PRIVATE_KEY_STAGING=' "$candidate"; then
      keys_file="$candidate"
      break
    fi
  done
  if [[ -n "$keys_file" ]]; then
    api_base=$(dotenvx get PUBLIC_API_BASE_URL -f .env.staging -fk "$keys_file" 2>/dev/null || true)
  else
    api_base=$(dotenvx get PUBLIC_API_BASE_URL -f .env.staging 2>/dev/null || true)
  fi
fi
api_base="${api_base%/}"
if [[ ! "$api_base" =~ ^https://[^/]+$ ]]; then
  echo "error: staging API base URL not found. Set MONACO_API_BASE_URL, or add DOTENV_PRIVATE_KEY_STAGING to .env.keys." >&2
  exit 1
fi

# The Grafana token reaches curl through a mode-600 config file, never argv, and
# the file is removed on exit.
curl_cfg="$(umask 077; mktemp)"
trap 'rm -f "$curl_cfg"' EXIT
(umask 077; printf 'header = "Authorization: Bearer %s"\n' "$GRAFANA_SERVICE_ACCOUNT_TOKEN" > "$curl_cfg")

# gcurl <method> <path> [json-body-on-stdin]: prints the body, then the status code on its own line.
gcurl() {
  curl -sS -K "$curl_cfg" -X "$1" -H 'Content-Type: application/json' \
    --data-binary @- -w '\n%{http_code}' "$grafana_url$2"
}
# call <method> <path> <expected-codes-regex> [stdin]: fails with the status and body unless the code matches.
call() {
  local out code
  out="$(gcurl "$1" "$2")"
  code="${out##*$'\n'}"
  if [[ ! "$code" =~ ^($3)$ ]]; then
    echo "error: $1 $2 answered $code: ${out%$'\n'*}" >&2
    return 1
  fi
  printf '%s' "${out%$'\n'*}"
}
folder_uid="$(jq -r .uid ops/grafana/folder.json)"
# A folder is created once. Grafana answers 409 or 412 when the uid already exists, which is fine.
folder_out="$(gcurl POST /api/folders < ops/grafana/folder.json)"
case "${folder_out##*$'\n'}" in
  200|409|412) ;;
  *) echo "error: POST /api/folders answered ${folder_out##*$'\n'}: ${folder_out%$'\n'*}" >&2; exit 1 ;;
esac
echo "folder: $(jq -r .title ops/grafana/folder.json)"

# shellcheck disable=SC2016 # $ENV is a jq variable, not a shell one
ds_filter='.url |= sub("__MONACO_API_BASE_URL__"; $ENV.api_base) | .jsonData.allowedHosts |= map(sub("__MONACO_API_BASE_URL__"; $ENV.api_base))'
export api_base
ds_body=$(jq "$ds_filter" ops/grafana/datasource.json)
if [[ "$set_token" == 1 ]]; then
  IFS= read -r MONACO_ADMIN_TOKEN || true
  if [[ -z "${MONACO_ADMIN_TOKEN:-}" ]]; then
    echo "error: --set-token reads the service token from stdin and got nothing" >&2
    exit 1
  fi
  export MONACO_ADMIN_TOKEN
  ds_body=$(jq '. + {secureJsonData: {httpHeaderValue1: ("Bearer " + $ENV.MONACO_ADMIN_TOKEN)}}' <<<"$ds_body")
  unset MONACO_ADMIN_TOKEN
fi
ds_uid="$(jq -r .uid ops/grafana/datasource.json)"
# Update in place by uid, else create. An update without a token keeps the secure header Grafana holds.
ds_out="$(gcurl PUT "/api/datasources/uid/$ds_uid" <<<"$ds_body")"
case "${ds_out##*$'\n'}" in
  200) ;;
  404)
    call POST /api/datasources 200 <<<"$ds_body" >/dev/null
    if [[ "$set_token" != 1 ]]; then
      echo "warning: data source created without a token. Run with --set-token to store one." >&2
    fi
    ;;
  *) echo "error: PUT /api/datasources/uid/$ds_uid answered ${ds_out##*$'\n'}: ${ds_out%$'\n'*}" >&2; exit 1 ;;
esac
unset ds_body
echo "data source: $(jq -r .name ops/grafana/datasource.json)"

for file in ops/grafana/dashboards/*.json; do
  [[ -e "$file" ]] || continue
  call POST /api/dashboards/db 200 < <(
    jq --arg folder "$folder_uid" \
      '{dashboard: (. + {id: null}), folderUid: $folder, overwrite: true, message: "grafana-sync"}' "$file"
  ) >/dev/null
  echo "dashboard: $(jq -r '.title' "$file") ($(jq -r .uid "$file"))"
done
