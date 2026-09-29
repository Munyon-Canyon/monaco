#!/usr/bin/env bash
# Prints "<feature> <N>" when <ref> is a feature branch <feature>-checkpoint-<N>, or the legacy <feature>-<N>,
# and exits 1 otherwise.
# The same pattern lives in scripts/agent-guard.py and scripts/check-pr-format.py; scripts/feature_branch_test.go keeps them equal.
set -euo pipefail

FEATURE_BRANCH_RE='^[a-z0-9]+(-[a-z0-9]+)*-[0-9]+$'

[[ $# -eq 1 ]] || { echo "usage: scripts/ci/feature-branch-name.sh <ref>" >&2; exit 2; }
[[ "$1" =~ $FEATURE_BRANCH_RE ]] || exit 1
feature="${1%-*}"
echo "${feature%-checkpoint} ${1##*-}"
