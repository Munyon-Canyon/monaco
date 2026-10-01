#!/usr/bin/env bash
# Run one command while holding a machine-wide build lock, one lock per class.
#
# Several xcodebuild / swift-build processes at once exhaust memory on a 16 GB Mac
# (each takes several GB) and the simulators start crashing. Heavy local steps go
# through this wrapper, and the class says which limit applies:
#
#   scripts/qa/xcode-lock.sh xcode xcodebuild -project ... test   # one at a time (3 to 7 GB each)
#   scripts/qa/xcode-lock.sh swiftpm swift test                   # SwiftPM builds (1 to 1.5 GB each)
#   scripts/qa/xcode-lock.sh xcodebuild -project ... test         # no class: same as `xcode`
#
# The classes are independent: an `xcode` holder and a `swiftpm` holder run together.
# Go, lint and shell steps need no lock.
#
# The lock is a directory (mkdir is atomic). Waiters queue first-come first-served:
# each writes a ticket `<lockdir>.queue/<epoch-ns>.<pid>` and takes the lock only when
# its ticket is the oldest live one. A ticket or a lock whose pid is gone is stale
# (`kill -0`) and is pruned or taken over. The holder records `pid` and `cwd` in the
# lock dir, and a waiter logs its queue position and the holder's cwd every 60 s.
#
# Environment:
#   MONACO_XCODE_LOCK_DIR      `xcode` lock dir (default /private/tmp/monaco-xcodebuild.lock)
#   MONACO_SWIFTPM_LOCK_DIR    `swiftpm` lock dir (default /private/tmp/monaco-swiftpm.lock)
#   MONACO_XCODE_LOCK_TIMEOUT  seconds a waiter waits before exit 75 (default 5400)
#   MONACO_LOCK_HOLD           seconds the command may run before it is stopped and the
#                              script exits 124 (default 1800)
#   MONACO_LOCK_POLL           seconds between checks while waiting (default 2)
set -euo pipefail

case "${1:-}" in
  xcode | swiftpm) class="$1"; shift ;;
  *) class=xcode ;;
esac

if [[ $# -eq 0 ]]; then
  echo "usage: $0 [xcode|swiftpm] <command> [args...]" >&2
  exit 2
fi

case "$class" in
  xcode) lock_dir="${MONACO_XCODE_LOCK_DIR:-/private/tmp/monaco-xcodebuild.lock}" ;;
  swiftpm) lock_dir="${MONACO_SWIFTPM_LOCK_DIR:-/private/tmp/monaco-swiftpm.lock}" ;;
esac
queue_dir="$lock_dir.queue"
wait_limit="${MONACO_XCODE_LOCK_TIMEOUT:-5400}"
hold_cap="${MONACO_LOCK_HOLD:-1800}"
poll="${MONACO_LOCK_POLL:-2}"

say() { echo "xcode-lock($class): $*" >&2; }

# Epoch nanoseconds. BSD date has no %N and prints a literal N, so fall back to python3
# and then to whole seconds.
now_ns() {
  local t
  t="$(date +%s%N 2>/dev/null || true)"
  if [[ "$t" =~ ^[0-9]+$ ]]; then
    echo "$t"
  elif type -P python3 >/dev/null; then
    python3 -c 'print(__import__("time").time_ns())'
  else
    echo "$(date +%s)000000000"
  fi
}

have_lock=0
ticket=""
running=0       # 1 while the held command (or its limiter) may be alive
child=""        # pid of the command, or of the `timeout` that wraps it
watchdog=""     # pid of the shell watchdog (no-`timeout` path only)
cap_mark=""     # per-run file the watchdog writes when the cap fires
sig_name=""     # INT or TERM, once one arrived while the command ran
sig_rc=0

# Leave no ticket, no watchdog and, if this process took the lock, no lock behind. The
# EXIT trap runs it on any exit; the calls before `exit` make the normal paths explicit.
# It is idempotent. It never runs while the command is alive: a signal is forwarded to
# the command and waited for first (the INT and TERM traps, run_capped).
release() {
  if [[ -n "$watchdog" ]]; then
    pkill -P "$watchdog" 2>/dev/null || true
    kill "$watchdog" 2>/dev/null || true
    watchdog=""
  fi
  if [[ -n "$ticket" ]]; then
    rm -f "$queue_dir/$ticket"
  fi
  if (( have_lock )); then
    rm -rf "$lock_dir"
    have_lock=0
  fi
}

# Live tickets, oldest first. Tickets whose pid is gone are pruned on the way.
live_tickets() {
  local path t
  for path in "$queue_dir"/*; do
    if [[ -e "$path" ]]; then
      echo "${path##*/}"
    fi
  done | sort -t. -k1,1n -k2,2n | while read -r t; do
    if kill -0 "${t#*.}" 2>/dev/null; then
      echo "$t"
    else
      rm -f "$queue_dir/$t"
    fi
  done
}

# Run the command under the hold cap. `timeout` is GNU coreutils; a Mac has it only as
# `gtimeout` (Homebrew coreutils), so a plain shell watchdog covers the rest.
#
# The command (or the limiter) runs in the background and this function waits for it, so a
# trapped INT or TERM can be forwarded. GNU `timeout` moves itself and the command into a
# new process group, so a terminal's Ctrl-C no longer reaches the command; the forwarded
# signal does, because `timeout` relays it. After a signal the wait goes on until the
# command is gone, and only then does the caller release the lock.
run_capped() {
  local cap="$1"; shift
  local limiter rc=0
  limiter="$(type -P timeout || type -P gtimeout || true)"
  running=1
  # Bash applies `set -e` to the handler of a trap that interrupts `wait` and exits, which
  # would release the lock under the running command. Errexit stays off until it is gone.
  set +e
  if [[ -n "$limiter" ]]; then
    "$limiter" "$cap" "$@" <&0 &
    child=$!
  else
    # A background job of a non-interactive shell starts with SIGINT ignored, and bash 3.2
    # (stock macOS) cannot undo that with `trap - INT` (4.4 and later can). Start the command
    # through python3, which sets SIGINT back to the default before it execs, so Ctrl-C and
    # a forwarded INT stop the command on any bash. python3 is already used by now_ns.
    python3 -c 'import os, signal, sys; signal.signal(signal.SIGINT, signal.SIG_DFL); os.execvp(sys.argv[1], sys.argv[1:])' "$@" <&0 &
    child=$!
    cap_mark="$lock_dir/capped.$$"
    ( sleep "$cap" && : > "$cap_mark" && kill -TERM "$child" 2>/dev/null ) >/dev/null 2>&1 &
    watchdog=$!
  fi
  if [[ -n "$sig_name" ]]; then
    kill -s "$sig_name" "$child" 2>/dev/null || true
  fi
  wait "$child"
  rc=$?
  while (( running )) && kill -0 "$child" 2>/dev/null; do
    wait "$child"
    rc=$?
  done
  set -e
  running=0
  child=""
  if [[ -n "$watchdog" ]]; then
    pkill -P "$watchdog" 2>/dev/null || true
    kill "$watchdog" 2>/dev/null || true
    wait "$watchdog" 2>/dev/null || true
    watchdog=""
    if [[ -e "$cap_mark" && -z "$sig_name" ]]; then
      return 124
    fi
  fi
  return "$rc"
}

mkdir -p "$queue_dir"
ticket="$(now_ns).$$"
: > "$queue_dir/$ticket"
trap release EXIT
# INT and TERM. While waiting in the queue, just exit. While the command runs, pass the
# signal on and let run_capped wait for the command to exit, so the lock is never released
# under a live build. (Inline, because ShellCheck 0.11 flags a function used only by a trap.)
trap 'sig_name=INT; sig_rc=130; if (( running )); then if [[ -n "$child" ]]; then kill -s INT "$child" 2>/dev/null || true; fi; else exit 130; fi' INT
trap 'sig_name=TERM; sig_rc=143; if (( running )); then if [[ -n "$child" ]]; then kill -s TERM "$child" 2>/dev/null || true; fi; else exit 143; fi' TERM

start=$SECONDS
next_log=0
while true; do
  tickets="$(live_tickets)"
  oldest="$(head -n1 <<< "$tickets")"
  owner="$(cat "$lock_dir/pid" 2>/dev/null || true)"
  if [[ "$oldest" == "$ticket" ]]; then
    if mkdir "$lock_dir" 2>/dev/null; then
      have_lock=1
      break
    fi
    if [[ -n "$owner" ]] && ! kill -0 "$owner" 2>/dev/null; then
      say "taking over stale lock from pid $owner"
      rm -rf "$lock_dir"
      continue
    fi
  fi
  waited=$((SECONDS - start))
  if (( waited >= wait_limit )); then
    say "gave up after ${wait_limit}s waiting for pid ${owner:-unknown}"
    release
    exit 75
  fi
  if (( waited >= next_log )); then
    position="$(awk -v t="$ticket" '$0 == t { print NR }' <<< "$tickets")"
    holder_cwd="$(cat "$lock_dir/cwd" 2>/dev/null || true)"
    say "queue position ${position:-?} behind pid ${owner:-unknown} (${holder_cwd:-unknown}) (${waited}s)"
    next_log=$((waited-waited%60+60))
  fi
  sleep "$poll"
done

rm -f "$queue_dir/$ticket"
ticket=""
# cwd first: a waiter treats a lock with no pid yet as live, never as stale.
printf '%s\n' "$PWD" > "$lock_dir/cwd"
echo "$$" > "$lock_dir/pid"

rc=0
run_capped "$hold_cap" "$@" || rc=$?
if [[ -n "$sig_name" ]]; then
  # The command is gone; leave with the signal's code, not the command's.
  release
  exit "$sig_rc"
fi
if (( rc == 124 )); then
  say "command exceeded the ${hold_cap}s hold cap"
fi
release
exit "$rc"
