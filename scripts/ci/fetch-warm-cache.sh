#!/usr/bin/env bash
# Download the newest non-expired artifact named $1 from a successful
# ci-warm.yml push on FEATURE_BRANCH or main, and extract it into $2.
# Prints "artifact run <id>", or "nothing" when no run qualifies.
# A pull_request or merge_group artifact is never selected.
set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "usage: fetch-warm-cache.sh <artifact-name> <dest>" >&2
  exit 2
fi

name="$1"
dest="$2"
repo="${GITHUB_REPOSITORY:?}"

list="$(
  gh api --paginate \
    --jq '.artifacts[] | select(.expired == false) | [.created_at, .name, (.workflow_run.id | tostring)] | @tsv' \
    "repos/${repo}/actions/artifacts?per_page=100&name=${name}"
)"

picked=""
while IFS=$'\t' read -r _created got_name run_id; do
  [[ -n "${run_id:-}" ]] || continue
  [[ "$got_name" == "$name" ]] || continue
  info="$(
    gh api --jq '[.path, .event, .conclusion, (.head_branch // "")] | @tsv' \
      "repos/${repo}/actions/runs/${run_id}"
  )"
  IFS=$'\t' read -r path event conclusion branch <<<"$info"
  [[ "$path" == ".github/workflows/ci-warm.yml" ]] || continue
  # push only: drop pull_request and merge_group runs.
  [[ "$event" == "push" ]] || continue
  [[ "$conclusion" == "success" ]] || continue
  if [[ "$branch" == "main" ]]; then
    :
  elif [[ -n "${FEATURE_BRANCH:-}" && "$branch" == "$FEATURE_BRANCH" ]]; then
    :
  else
    continue
  fi
  picked="$run_id"
  break
done < <(printf '%s\n' "$list" | sort -r)

if [[ -z "$picked" ]]; then
  echo nothing
  exit 0
fi

mkdir -p "$dest"
gh run download "$picked" --repo "$repo" --name "$name" --dir "$dest" >&2
echo "artifact run $picked"
