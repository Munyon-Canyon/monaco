#!/bin/bash
# Usage: post-verify.sh <pr> <sha> <success|failure> "<description>"
# Posts the `verify` commit status: as the monaco-verifier App when its key is readable, else with your gh token. Refuses unless <sha> is the PR's current head.
# Never prints the key or tokens.
set -euo pipefail
pr=$1 sha=$2 state=$3 desc=$4
head=$(gh pr view "$pr" --json headRefOid -q .headRefOid)
[[ "$head" == "$sha"* ]] || { echo "post-verify.sh: REFUSED: $sha is not #$pr head ($head)" >&2; exit 1; }
body=$(jq -nc --arg s "$state" --arg d "${desc:0:140}" '{state:$s,context:"verify",description:$d}')
key="$HOME/.config/monaco/verifier.pem"
if [[ -r "$key" ]]; then
  jwt=$($(dirname "$0")/../app-jwt.sh 5101392 "$key")
  tok=$(curl -fsS -X POST -H "Authorization: Bearer $jwt" -H "Accept: application/vnd.github+json" \
    https://api.github.com/app/installations/165592710/access_tokens | jq -r .token)
  curl -fsS -X POST -H "Authorization: Bearer $tok" -H "Accept: application/vnd.github+json" \
    "https://api.github.com/repos/lognorman20/monaco/statuses/$head" -d "$body" | jq -r '"verify=\(.state) on \(.url|split("/")|last|.[0:8]) by \(.creator.login)"'
else
  gh api -X POST "repos/lognorman20/monaco/statuses/$head" --input - <<<"$body" --jq '"verify=\(.state) on \(.url|split("/")|last|.[0:8]) by \(.creator.login)"'
fi
