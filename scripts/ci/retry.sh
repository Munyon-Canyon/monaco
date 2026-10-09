#!/usr/bin/env bash
# Retries a command with a growing, jittered delay. Anonymous pulls from public.ecr.aws are
# rate limited per source address, and jobs that start in the same second fail with
# "toomanyrequests: Rate exceeded". The jitter keeps the jobs that failed together from retrying together.
set -uo pipefail

attempts="${RETRY_ATTEMPTS:-5}"
unit="${RETRY_SLEEP:-5}"

for ((attempt = 1; ; attempt++)); do
  "$@" && exit 0
  status=$?
  ((attempt >= attempts)) && exit "$status"
  delay=$((unit * attempt))
  ((unit > 0)) && delay=$((delay + RANDOM % unit))
  echo "retry: attempt $attempt of $attempts failed with $status, retrying in ${delay}s: $*" >&2
  sleep "$delay"
done
