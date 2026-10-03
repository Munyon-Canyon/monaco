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
  "$@" 2>&1 | tee "$log"
  rc=${PIPESTATUS[0]}
  set -e
  if [[ "$rc" -eq 0 ]]; then
    cp "$log" "$pass"
  fi
  return "$rc"
}

summary='select((.Action == "output" and .Test == null and (.Output | test("^(PASS|-test\\.shuffle |coverage: )") | not))
  or .Action == "build-output") | .Output'

print_failures() {
  echo
  echo "--- output of failed tests ---"
  jq -rsj '[.[] | select(.Action == "fail") | "\(.Package) \(.Test // "")"] as $failed
    | .[] | select(.Action == "output")
    | select(if .Test == null then (.Output | startswith("-test.shuffle ")) and ("\(.Package) " | IN($failed[]))
      else "\(.Package) \(.Test)" | IN($failed[]) end)
    | .Output' "$@"
}

# shard_packages i n table [pattern...] prints the packages of shard i of n over the patterns (default ./...).
# Packages go to the least loaded shard, heaviest first, weighed by table (package<TAB>seconds); a package
# missing from it weighs 5 s.
shard_packages() {
  local i="$1" n="$2" table="$3"
  shift 3
  go list "${@:-./...}" | sort | awk -v i="$i" -v n="$n" -v table="$table" -f "$script_dir/test-shard.awk"
}

# shard_list i n table pattern... fills the caller's pkgs array with shard i of n, or with the patterns when n is 1.
shard_list() {
  local i="$1" n="$2" table="$3" list line
  shift 3
  if [[ "$n" -eq 1 ]]; then
    pkgs=("$@")
    return 0
  fi
  list="$(shard_packages "$i" "$n" "$table" "$@")"
  while IFS= read -r line; do
    if [[ -n "$line" ]]; then
      pkgs+=("$line")
    fi
  done <<<"$list"
  if [[ "${#pkgs[@]}" -eq 0 ]]; then
    echo "error: shard $i/$n has no packages." >&2
    return 1
  fi
}

short_pass() {
  local i="$1" n="$2" name json cover pkgs=() status=0
  shift 2
  name="short-$i-of-$n"
  json="$out/$name.json"
  cover="$out/$name.cover"
  shard_list "$i" "$n" .test-shards.tsv ./... || return 1
  # -p 4: at the default -p 8, eight test binaries each run their parallel tests at once and starve each
  # other; packages that take 3 s alone went over the 10 s package budget.
  go test -json -tags faultpoints -race -shuffle=on -short -p 4 -coverpkg=./... -coverprofile="$cover" "$@" "${pkgs[@]}" |
    tee "$json" | jq -rj --unbuffered "$summary" || status=1
  if [[ "$status" -ne 0 ]]; then
    print_failures "$json"
  fi
  return "$status"
}

# rest_pass i n runs shard i of n of the non-short pass, split by .test-shards-full.tsv. Shard 1 also runs the
# faultpoint package without its build tag and the allocation baselines.
rest_pass() {
  local i="$1" n="$2" json="$out/rest.json" json_full cover_full status=0 allocs=() pkgs=() dir files=()
  shift 2
  json_full="$out/full-$i-of-$n.json"
  cover_full="$out/full-$i-of-$n.cover"
  if [[ "$i" -eq 1 ]]; then
    files+=("$json")
    go test -json -race -short -coverpkg=./internal/platform/faultpoint/ -coverprofile="$out/rest-faultpoint.cover" "$@" \
      ./internal/platform/faultpoint/ | tee "$json" | jq -rj --unbuffered "$summary" || status=1

    # -race makes sync.Pool drop items at random, so allocation baselines run in a second pass without it.
    while IFS= read -r dir; do
      allocs+=("$dir")
    done < <(find . -name allocs_test.go -not -path '*/testdata/*' -exec dirname {} \; | sort -u)
    if [[ "${#allocs[@]}" -gt 0 ]]; then
      go test -json -short -run '^TestAllocs' "${allocs[@]}" | tee -a "$json" | jq -rj --unbuffered "$summary" || status=1
    fi
  fi

  shard_list "$i" "$n" .test-shards-full.tsv ./internal/platform/ids/... ./internal/platform/lint/... \
    ./internal/testkit/... ./internal/tools/gen/... ./cmd/monacoctl ./cmd/monacoctl/verify || return 1
  files+=("$json_full")
  go test -json -race -coverpkg=./... -coverprofile="$cover_full" "$@" "${pkgs[@]}" |
    tee "$json_full" | jq -rj --unbuffered "$summary" || status=1
  if [[ "$status" -ne 0 ]]; then
    print_failures "${files[@]}"
  fi
  return "$status"
}

# report_pass [start] merges every pass under $out and gates it. The full-run packages are exempt from the
# per-package budget, so every full*.json goes to --budget-exempt and every other *.json to --from.
report_pass() {
  local status=0 json="$work/go-test.json" exempt="$work/full.json" cover="$work/cover.out" f have_json=''
  local have_exempt='' have_cover='' report
  : >"$json"
  : >"$exempt"
  for f in "$out"/*.json; do
    if [[ ! -e "$f" ]]; then
      continue
    fi
    case "$(basename "$f")" in
      full*.json)
        cat "$f" >>"$exempt"
        have_exempt=1
        ;;
      *)
        cat "$f" >>"$json"
        have_json=1
        ;;
    esac
  done
  for f in "$out"/*.cover; do
    if [[ -e "$f" ]]; then
      if [[ -z "$have_cover" ]]; then
        cat "$f" >"$cover"
      else
        tail -n +2 "$f" >>"$cover"
      fi
      have_cover=1
    fi
  done
  if [[ -z "$have_json" || -z "$have_cover" ]]; then
    echo "error: no test output under $out. Run the short and rest passes first." >&2
    return 1
  fi
  report=(--from "$json")
  if [[ -n "$have_exempt" ]]; then
    report+=(--budget-exempt "$exempt")
  fi
  if [[ -n "${1:-}" ]]; then
    report+=(--start "$1")
  fi
  if [[ -n "${CI:-}" ]]; then
    report+=(--ci)
  fi
  go run ./cmd/monacoctl test-report "${report[@]}" || status=1
  go run ./cmd/monacoctl flows check --from "$json" || status=1
  go run ./cmd/monacoctl coverage --profile "$cover" | tail -n 40 || status=1
  return "$status"
}

run_suite() {
  set -o pipefail
  if ! command -v jq >/dev/null 2>&1; then
    echo "error: jq is missing. Install it (brew install jq) and rerun." >&2
    return 1
  fi

  script_dir="$(cd "$(dirname "$0")" && pwd)"
  cd "$script_dir/../apps/backend"
  work="$(mktemp -d)"
  git_dir="$(mktemp -d)"
  out="${TEST_BACKEND_OUT:-$work/out}"
  mkdir -p "$out"
  trap 'rm -rf "$work" "$git_dir"' EXIT

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

  local status=0 start shard pass="${TEST_BACKEND_PASS:-}"
  case "$pass" in
    "")
      start="$(date +%s)"
      short_pass 1 1 "$@" || status=1
      rest_pass 1 1 "$@" || status=1
      report_pass "$start" || status=1
      ;;
    short:[1-9]*/[1-9]*)
      shard="${pass#short:}"
      if [[ "${shard%/*}" -gt "${shard#*/}" ]]; then
        echo "error: TEST_BACKEND_PASS=$pass: the shard index is past the shard count." >&2
        return 2
      fi
      short_pass "${shard%/*}" "${shard#*/}" "$@" || status=1
      ;;
    rest:[1-9]*/[1-9]*)
      shard="${pass#rest:}"
      if [[ "${shard%/*}" -gt "${shard#*/}" ]]; then
        echo "error: TEST_BACKEND_PASS=$pass: the shard index is past the shard count." >&2
        return 2
      fi
      rest_pass "${shard%/*}" "${shard#*/}" "$@" || status=1
      ;;
    rest) rest_pass 1 1 "$@" || status=1 ;;
    report) report_pass || status=1 ;;
    *)
      echo "error: TEST_BACKEND_PASS=$pass is not short:<i>/<n>, rest, rest:<i>/<n> or report." >&2
      return 2
      ;;
  esac
  return "$status"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  if [[ -n "${TEST_BACKEND_CACHE_EXEC:-}" ]]; then
    run_cached bash -c "$TEST_BACKEND_CACHE_EXEC"
    exit $?
  fi
  if [[ -n "${TEST_BACKEND_PASS:-}" ]]; then
    run_suite "$@"
    exit $?
  fi
  run_cached run_suite "$@"
fi
