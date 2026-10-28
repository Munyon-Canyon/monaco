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

run() {
  mkdir -p "$state"
  exec 9>>"$lock"
  if ! try_lock 9; then
    echo "another watch daemon holds ${lock}" >&2
    exit 1
  fi
  echo "$$" >"$pidfile"
  local log child=""
  log="$(logfile)"
  trap 'if [[ -n "$child" ]]; then kill "$child" 2>/dev/null || true; fi; rm -f "$pidfile"; exit 0' TERM INT
  if [[ ! -x "$bin" && -z "${MONACO_WATCH_BIN:-}" ]]; then
    (cd "${root}/apps/backend" && go build -o "$bin" ./cmd/monacoctl)
  fi
  while true; do
    rotate "$log"
    started="$(date +%s)"
    printf '%s daemon: starting %s agents watch\n' "$(date -u +%FT%TZ)" "$bin" >>"$log"
    "$bin" agents watch >>"$log" 2>&1 9>&- &
    child=$!
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
