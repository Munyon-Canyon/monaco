#!/usr/bin/env bash
# Keep the newest Actions cache on <ref> for each key prefix and delete the rest.
# A GitHub list or delete error is printed and ignored: the warm run already
# saved its caches, and this job must not fail the workflow. A key that does
# not start with the prefix is never deleted, and neither is an entry on
# another ref. Artifacts are not caches and are not listed here.
#
# prune-caches.sh --budget instead warns when every cache on the repository totals more than 7 GB. GitHub keeps
# 10 GB and evicts the least recently used entry when a save passes it, so the warning shows growth before an
# eviction. It always exits 0.
set -euo pipefail

if [[ "${1:-}" == "--budget" ]]; then
  limit=$((7 * 1024 * 1024 * 1024))
  if ! all="$(gh cache list --limit 1000 --json key,sizeInBytes 2>&1)"; then
    echo "budget: list failed: $(tr '\n' ' ' <<<"$all")" >&2
    exit 0
  fi
  total="$(jq '[.[].sizeInBytes // 0] | add // 0' <<<"$all")"
  gb="$(awk -v b="$total" 'BEGIN { printf "%.2f", b / 1073741824 }')"
  echo "caches total ${gb} GB"
  if ((total > limit)); then
    largest="$(jq -r '
      sort_by(-(.sizeInBytes // 0)) | .[:5]
      | map("\(.key) \(((.sizeInBytes // 0) / 1048576) | floor) MB") | join(", ")
    ' <<<"$all")"
    echo "::warning::Actions caches total ${gb} GB, over the 7 GB budget of the 10 GB limit. Largest: ${largest}"
  fi
  exit 0
fi

if [[ $# -lt 2 ]]; then
  echo "usage: prune-caches.sh <ref> <prefix>..." >&2
  exit 2
fi

ref="$1"
shift

if [[ -z "$ref" ]]; then
  echo "prune: empty ref" >&2
  exit 2
fi
for prefix in "$@"; do
  if [[ -z "$prefix" ]]; then
    echo "prune: an empty prefix would match every cache" >&2
    exit 2
  fi
done

deleted=0
bytes=0

prune_prefix() {
  local prefix="$1"
  local list errfile tsv
  errfile="$(mktemp)"
  # The default page is 30. One prefix on a busy ref can pass that, and the
  # page is the newest by created_at so the entry we keep is on it.
  if ! list="$(gh cache list --ref "$ref" --key "$prefix" --limit 200 --sort created_at --order desc --json id,key,createdAt,sizeInBytes,ref 2>"$errfile")"; then
    echo "prune: list failed for ${prefix}: $(tr '\n' ' ' <"$errfile")" >&2
    rm -f "$errfile"
    return 0
  fi
  rm -f "$errfile"

  # Ascending createdAt, so the newest row is last. No reverse filter: the
  # tool manifest reads jq text as shell.
  if ! tsv="$(jq -r --arg ref "$ref" '
    [ .[] | select(.ref == $ref) ] | sort_by(.createdAt // "0") | .[]
    | [(.id | tostring), ((.sizeInBytes // 0) | tostring), .ref, .key] | @tsv
  ' <<<"$list")"; then
    echo "prune: parse failed for ${prefix}" >&2
    return 0
  fi

  local -a ids sizes keys
  ids=()
  sizes=()
  keys=()
  local id size gotref key
  while IFS=$'\t' read -r id size gotref key; do
    [[ -n "${id:-}" ]] || continue
    [[ "$id" =~ ^[0-9]+$ ]] || continue
    [[ "$size" =~ ^[0-9]+$ ]] || size=0
    if [[ "$gotref" != "$ref" ]]; then
      continue
    fi
    # Prefix guard. gh --key is a prefix filter; a row outside it is never deleted.
    if [[ "$key" != "$prefix"* ]]; then
      continue
    fi
    ids+=("$id")
    sizes+=("$size")
    keys+=("$key")
  done <<<"$tsv"

  local i last
  last=$((${#ids[@]} - 1))
  for ((i = 0; i < last; i++)); do
    if ! gh cache delete "${ids[$i]}" >/dev/null; then
      echo "prune: delete ${ids[$i]} failed" >&2
      continue
    fi
    echo "deleted ${ids[$i]} ${keys[$i]}"
    deleted=$((deleted + 1))
    bytes=$((bytes + ${sizes[$i]}))
  done
}

for prefix in "$@"; do
  prune_prefix "$prefix"
done

mb=$((bytes / 1048576))
echo "pruned ${deleted} entries, ${mb} MB"
