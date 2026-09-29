#!/usr/bin/env bash
set -euo pipefail

FEATURE_BRANCH_RE='^[a-z0-9]+(-[a-z0-9]+)*-checkpoint-[0-9]+$'

[[ $# -eq 1 ]] || { echo "usage: scripts/ci/feature-branch-name.sh <ref>" >&2; exit 2; }
[[ "$1" =~ $FEATURE_BRANCH_RE ]] || exit 1
echo "${1%-checkpoint-*} ${1##*-}"
