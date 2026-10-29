#!/usr/bin/env bash
# Keep one `monacoctl agents watch` alive per clone: `start`, `stop`, `status`.
# The daemon holds <git common dir>/.monaco/watch-daemon.lock for its whole life, so the lock
# stays held while the watch restarts. `land-stack` reads that lock and the watch's own watch.lock.
set -euo pipefail

root="$(git rev-parse --show-toplevel)"
common="$(git rev-parse --path-format=absolute --git-common-dir)"
state="${common}/.monaco"
lock="${state}/watch-daemon.lock"
pidfile="${state}/watch-daemon.pid"
bin="${MONACO_WATCH_BIN:-${root}/bin/monacoctl}"
restart_after="${MONACO_WATCH_RESTART_AFTER:-2}"
stale_poll="${MONACO_WATCH_STALE_POLL:-60}"
source_dir="apps/backend/cmd/monacoctl"
max_log_bytes="${MONACO_WATCH_LOG_BYTES:-5242880}"

milestone() {
  local f m
  for f in "${state}/agents.local.toml" "${root}/.monaco/agents.toml"; do
    m="$(sed -n 's/^milestone *= *"\(.*\)".*/\1/p' "$f" 2>/dev/null | head -n 1)"
    if [[ -n "$m" ]]; then
      echo "$m"
      return
    fi
  done
  echo "watch"
}

logfile() {
  local dir
  dir="${common}/pstack/$(milestone)/logs"
  mkdir -p "$dir"
  echo "${dir}/watch-daemon.log"
}

# Take the exclusive lock on the open file descriptor $1; it is released when the last holder of that descriptor exits.
try_lock() {
  python3 -c 'import fcntl, sys
try:
    fcntl.flock(int(sys.argv[1]), fcntl.LOCK_EX | fcntl.LOCK_NB)
except OSError:
    sys.exit(75)' "$1"
}

# Exits 0 when no daemon holds the lock.
lock_free() {
  mkdir -p "$state"
  (
    exec 8>>"$lock"
    try_lock 8
  )
}

rotate() {
  local log="$1" size
  size="$(wc -c <"$log" 2>/dev/null || echo 0)"
  if ((size > max_log_bytes)); then
    mv -f "$log.2" "$log.3" 2>/dev/null || true
    mv -f "$log.1" "$log.2" 2>/dev/null || true
    mv -f "$log" "$log.1"
  fi
}

source_commit() {
  git -C "$root" log -1 --format=%H -- "$source_dir"
}

# Exits 0 when the clone's binary was built from an older commit under cmd/monacoctl than HEAD has.
stale() {
  [[ -z "${MONACO_WATCH_BIN:-}" ]] || return 1
  [[ -x "$bin" ]] || return 0
  [[ "$(cat "${bin}.src" 2>/dev/null)" != "$(source_commit)" ]]
}

# Records the commit it tried, so a failed build keeps the old binary until the next change.
build() {
  local commit
  commit="$(source_commit)"
  if (cd "${root}/apps/backend" && go build -o "${bin}.new" ./cmd/monacoctl); then
    mv -f "${bin}.new" "$bin"
    printf '%s daemon: built %s from %s\n' "$(date -u +%FT%TZ)" "$bin" "$commit"
  else
    printf '%s daemon: build from %s failed; keeping the old binary\n' "$(date -u +%FT%TZ)" "$commit"
  fi
  echo "$commit" >"${bin}.src"
}

run() {
  mkdir -p "$state"
  exec 9>>"$lock"
  if ! try_lock 9; then
    echo "another watch daemon holds ${lock}" >&2
    exit 1
  fi
  echo "$$" >"$pidfile"
  local log child="" pause=""
  log="$(logfile)"
  trap 'kill $child $pause 2>/dev/null || true; rm -f "$pidfile"; exit 0' TERM INT
  while true; do
    rotate "$log"
    if stale; then
      build >>"$log" 2>&1
    fi
    started="$(date +%s)"
    printf '%s daemon: starting %s agents watch\n' "$(date -u +%FT%TZ)" "$bin" >>"$log"
    "$bin" agents watch >>"$log" 2>&1 9>&- &
    child=$!
    while kill -0 "$child" 2>/dev/null; do
      if stale; then
        printf '%s daemon: %s changed; restarting the watch on a new build\n' "$(date -u +%FT%TZ)" "$source_dir" >>"$log"
        kill "$child" 2>/dev/null || true
        break
      fi
      sleep "$stale_poll" 9>&- &
      pause=$!
      wait "$pause" || true
      pause=""
    done
    wait "$child" || true
    child=""
    printf '%s daemon: watch exited after %ss\n' "$(date -u +%FT%TZ)" "$(($(date +%s) - started))" >>"$log"
    if (($(date +%s) - started < 5)); then
      sleep 5
    else
      sleep "$restart_after"
    fi
  done
}

running_pid() {
  if lock_free; then
    return 1
  fi
  cat "$pidfile" 2>/dev/null || echo "unknown"
}

start() {
  local pid
  if pid="$(running_pid)"; then
    echo "watch daemon already running (pid ${pid}); stop it with just stop watch" >&2
    exit 1
  fi
  local log
  log="$(logfile)"
  nohup "$0" run >/dev/null 2>&1 &
  for _ in $(seq 1 50); do
    if pid="$(running_pid)" && [[ "$pid" != unknown ]]; then
      echo "watch daemon started (pid ${pid}); log ${log}"
      return
    fi
    sleep 0.1
  done
  echo "error: watch daemon did not start; see ${log}" >&2
  exit 1
}

stop() {
  local pid
  if ! pid="$(running_pid)"; then
    echo "no watch daemon is running"
    return
  fi
  kill -TERM "$pid"
  for _ in $(seq 1 100); do
    if lock_free; then
      echo "watch daemon stopped"
      return
    fi
    sleep 0.1
  done
  echo "error: watch daemon (pid ${pid}) still running 10s after SIGTERM" >&2
  exit 1
}

case "${1:-}" in
  start) start ;;
  stop) stop ;;
  run) run ;;
  status)
    if pid="$(running_pid)"; then echo "watch daemon running (pid ${pid})"; else echo "no watch daemon is running"; fi
    ;;
  *)
    echo "usage: $0 start|stop|status" >&2
    exit 2
    ;;
esac
