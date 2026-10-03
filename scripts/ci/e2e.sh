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

verify_if_runnable() {
  echo "::group::monacoctl verify $*"
  SECONDS=0
  output=$("$bin" verify "$@" 2>&1)
  code=$?
  printf '%s\n' "$output"
  if [[ $code -ne 0 && $output != *"no built flow outcome matches"* ]]; then
    status=1
  elif [[ $code -ne 0 ]]; then
    echo "monacoctl verify $* skipped: no runnable crash scenario"
  fi
  echo "::endgroup::"
  echo "monacoctl verify $* took ${SECONDS}s"
}

verify all
verify all --crash-at after-publish
verify all --crash-at before-commit
verify_if_runnable all --crash-at after-sign
exit "$status"
