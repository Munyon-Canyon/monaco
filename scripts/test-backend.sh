#!/usr/bin/env bash
set -euo pipefail

cache_root() {
  if [[ -n "${MONACO_TEST_BACKEND_CACHE:-}" ]]; then
    printf '%s\n' "$MONACO_TEST_BACKEND_CACHE"
    return
  fi
  local common top
  common="$(git rev-parse --git-common-dir 2>/dev/null || true)"
  if [[ -z "$common" ]]; then
    printf '\n'
    return
  fi
  if [[ "$common" != /* ]]; then
    top="$(git rev-parse --show-toplevel)"
    common="$top/$common"
  fi
  printf '%s\n' "$common/monaco-test-backend"
}

tree_id() {
  local top idx tree gitdir
  top="$(git rev-parse --show-toplevel)"
  gitdir="$(git rev-parse --git-dir)"
  idx="$(mktemp)"
  if [[ -f "$gitdir/index" ]]; then
    cp "$gitdir/index" "$idx"
  fi
  tree="$(cd "$top" && GIT_INDEX_FILE="$idx" git add -A && GIT_INDEX_FILE="$idx" git write-tree)"
  rm -f "$idx"
  printf '%s\n' "$tree"
}

run_cached() {
  local root tree key pass log rc
  root="$(cache_root)"
  if [[ -z "$root" ]]; then
    "$@"
    return
  fi
  mkdir -p "$root"
  tree="$(tree_id)"
  key="$(printf '%s\0%s' "$tree" "$*" | git hash-object --stdin)"
  pass="$root/$key.pass"
  if [[ -z "${TEST_BACKEND_FORCE:-}" && -f "$pass" ]]; then
    echo "test-backend: cached PASS for tree $tree"
    cat "$pass"
    return 0
  fi
  log="$root/$key.log"
  set +e
  set +o pipefail
  "$@" 2>&1 | tee "$log"
  rc=${PIPESTATUS[0]}
  set -o pipefail
  set -e
  if [[ "$rc" -eq 0 ]]; then
    cp "$log" "$pass"
  fi
  return "$rc"
}

run_suite() {
  set -o pipefail
  if ! command -v jq >/dev/null 2>&1; then
    echo "error: jq is missing. Install it (brew install jq) and rerun." >&2
    return 1
  fi

  cd "$(dirname "$0")/../apps/backend"
  json="$(mktemp)"
  json_full="$(mktemp)"
  cover="$(mktemp)"
  cover_off="$(mktemp)"
  cover_full="$(mktemp)"
  git_dir="$(mktemp -d)"
  trap 'rm -f "$json" "$json_full" "$cover" "$cover_off" "$cover_full"; rm -rf "$git_dir"' EXIT

  # rapid divides checks by 5 and steps by 2 under -short, so this lands on 100 cases and about 20 steps.
  # Env vars, not -rapid.* flags: a test binary that does not link rapid rejects the flags.
  export RAPID_CHECKS=500 RAPID_STEPS=40
  # On macOS /usr/bin/git is an xcrun trampoline that costs about 16 ms of CPU per call, three times
  # git's own work. Tests run git a few hundred times, so put the real binary first on PATH.
  if [[ "$(uname -s)" == Darwin ]] && git_bin="$(xcrun -f git 2>/dev/null)"; then
    ln -s "$git_bin" "$git_dir/git"
    export PATH="$git_dir:$PATH"
  fi
  if [[ -n "${CI:-}" ]]; then
    export RAPID_NOFAILFILE=1
  fi

  status=0
  start="$(date +%s)"
  summary='select((.Action == "output" and .Test == null and (.Output | test("^(PASS|-test\\.shuffle |coverage: )") | not))
    or .Action == "build-output") | .Output'
  # -p 4: at the default -p 8, eight test binaries each run their parallel tests at once and starve each
  # other; packages that take 3 s alone went over the 10 s package budget.
  go test -json -tags faultpoints -race -shuffle=on -short -p 4 -coverpkg=./... -coverprofile="$cover" "$@" ./... | tee "$json" | jq -rj --unbuffered "$summary" || status=1

  go test -json -race -short -coverpkg=./internal/platform/faultpoint/ -coverprofile="$cover_off" "$@" ./internal/platform/faultpoint/ |
    tee -a "$json" | jq -rj --unbuffered "$summary" || status=1
  tail -n +2 "$cover_off" >>"$cover"

  # -race makes sync.Pool drop items at random, so allocation baselines run in a second pass without it.
  allocs=()
  while IFS= read -r dir; do
    allocs+=("$dir")
  done < <(find . -name allocs_test.go -not -path '*/testdata/*' -exec dirname {} \; | sort -u)
  if [[ "${#allocs[@]}" -gt 0 ]]; then
    go test -json -short -run '^TestAllocs' "${allocs[@]}" | tee -a "$json" | jq -rj --unbuffered "$summary" || status=1
  fi

  go test -json -race -coverpkg=./... -coverprofile="$cover_full" "$@" ./internal/platform/ids/... \
    ./internal/platform/lint/... ./internal/testkit/... ./internal/tools/gen/... ./cmd/monacoctl ./cmd/monacoctl/verify |
    tee "$json_full" | jq -rj --unbuffered "$summary" || status=1
  tail -n +2 "$cover_full" >>"$cover"

  if [[ "$status" -ne 0 ]]; then
    echo
    echo "--- output of failed tests ---"
    jq -rsj '[.[] | select(.Action == "fail") | "\(.Package) \(.Test // "")"] as $failed
      | .[] | select(.Action == "output")
      | select(if .Test == null then (.Output | startswith("-test.shuffle ")) and ("\(.Package) " | IN($failed[]))
        else "\(.Package) \(.Test)" | IN($failed[]) end)
      | .Output' "$json" "$json_full"
  fi

  report=(--from "$json" --budget-exempt "$json_full" --start "$start")
  if [[ -n "${CI:-}" ]]; then
    report+=(--ci)
  fi
  go run ./cmd/monacoctl test-report "${report[@]}" || status=1
  go run ./cmd/monacoctl flows check --from "$json" || status=1
  go run ./cmd/monacoctl coverage --profile "$cover" | tail -n 40 || status=1
  return "$status"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  if [[ -n "${TEST_BACKEND_CACHE_EXEC:-}" ]]; then
    run_cached bash -c "$TEST_BACKEND_CACHE_EXEC"
    exit $?
  fi
  run_cached run_suite "$@"
fi
