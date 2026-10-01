#!/usr/bin/env bash
# Download the newest non-expired artifact named $1 from a successful
# ci-warm.yml push on FEATURE_BRANCH or main, in this repository, and
# extract its tar into $2. Prints "artifact run <id>", or "nothing" when
# no run qualifies or any GitHub call fails. A pull_request or merge_group
# artifact is never selected. A cache miss must not fail the job.
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
)" || { echo nothing; exit 0; }

picked=""
while IFS=$'\t' read -r _created got_name run_id; do
  [[ -n "${run_id:-}" ]] || continue
  [[ "$got_name" == "$name" ]] || continue
  info="$(
    gh api --jq '[.path, .event, .conclusion, (.head_branch // ""), (.head_repository.full_name // "")] | @tsv' \
      "repos/${repo}/actions/runs/${run_id}"
  )" || continue
  IFS=$'\t' read -r path event conclusion branch head_repo <<<"$info"
  [[ "$path" == ".github/workflows/ci-warm.yml" ]] || continue
  # push only: drop pull_request and merge_group runs.
  [[ "$event" == "push" ]] || continue
  [[ "$conclusion" == "success" ]] || continue
  [[ "$head_repo" == "$repo" ]] || continue
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

tmp="$(mktemp -d "${RUNNER_TEMP:-/tmp}/warm-cache.XXXXXX")"
if ! gh run download "$picked" --repo "$repo" --name "$name" --dir "$tmp" >&2; then
  rm -rf "$tmp"
  echo nothing
  exit 0
fi

tarfile=""
for f in "$tmp"/*.tar.zst "$tmp"/*.tar; do
  if [[ -f "$f" ]]; then
    tarfile="$f"
    break
  fi
done
base="$(basename "$dest")"
extract_ok=0
if [[ -n "$tarfile" ]]; then
  if [[ "$tarfile" == *.tar.zst ]]; then
    tar -C "$tmp" --use-compress-program='zstd -d' -xf "$tarfile" && extract_ok=1
  elif tar -C "$tmp" -xf "$tarfile"; then
    extract_ok=1
  fi
fi
if [[ "$extract_ok" -ne 1 || ! -d "$tmp/$base" ]]; then
  rm -rf "$tmp"
  echo nothing
  exit 0
fi

mkdir -p "$(dirname "$dest")"
rm -rf "$dest"
mv "$tmp/$base" "$dest"
rm -rf "$tmp"
echo "artifact run $picked"
