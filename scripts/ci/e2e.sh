#!/usr/bin/env bash
set -uo pipefail

leg="${E2E_LEG:-1/1}"
leg_index="${leg%/*}"
count="${leg#*/}"
if ! [[ "$leg_index" =~ ^[0-9]+$ && "$count" =~ ^[0-9]+$ ]] || [ "$leg_index" -lt 1 ] || [ "$leg_index" -gt "$count" ]; then
  echo "E2E_LEG must be i/n with 1 <= i <= n, got '$leg'" >&2
  exit 2
fi

cd "$(dirname "$0")/../../apps/backend" || exit 1
bin="${E2E_MONACOCTL:-${RUNNER_TEMP:-/tmp}/monacoctl}"
if [ -z "${E2E_MONACOCTL:-}" ]; then
  go build -o "$bin" ./cmd/monacoctl || exit 1
fi
status=0

verify() {
  echo "::group::monacoctl verify $*"
  SECONDS=0
  "$bin" verify "$@" || status=1
  echo "::endgroup::"
  echo "monacoctl verify $* took ${SECONDS}s"
}

if ! points=$("$bin" flows crash-points); then
  exit 1
fi
units=(all)
for point in $points; do
  units+=("$point")
done

j=-1
for unit in "${units[@]}"; do
  j=$((j + 1))
  [ $((j % count)) -eq $((leg_index - 1)) ] || continue
  if [ "$unit" = all ]; then
    verify all
  else
    verify all --crash-at "$unit"
  fi
done
exit "$status"
