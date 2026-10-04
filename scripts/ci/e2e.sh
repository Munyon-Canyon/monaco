#!/usr/bin/env bash
set -uo pipefail

cd "$(dirname "$0")/../../apps/backend" || exit 1
bin="${RUNNER_TEMP:-/tmp}/monacoctl"
go build -o "$bin" ./cmd/monacoctl || exit 1
status=0

verify() {
  echo "::group::monacoctl verify $*"
  SECONDS=0
  "$bin" verify "$@" || status=1
  echo "::endgroup::"
  echo "monacoctl verify $* took ${SECONDS}s"
}

verify all
if ! points=$("$bin" flows crash-points); then
  status=1
fi
for point in $points; do
  verify all --crash-at "$point"
done
exit "$status"
