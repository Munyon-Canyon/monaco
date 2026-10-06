#!/usr/bin/env bash
# Run one command while holding one of a class's machine-wide build slots.
#
# Too many xcodebuild / swift-build processes at once exhaust memory (each takes several
# GB) and the simulators start crashing. Heavy local steps go through this wrapper, and
# the class says which limit applies:
#
#   scripts/qa/xcode-lock.sh xcode xcodebuild -project ... test   # 3 to 7 GB each
#   scripts/qa/xcode-lock.sh swiftpm swift test                   # 1 to 1.5 GB each
#   scripts/qa/xcode-lock.sh xcodebuild -project ... test         # no class: same as `xcode`
#
# Each class has N slots, sized from physical RAM unless set: `xcode` one per 16 GB,
# `swiftpm` one per 8 GB, at least 1. A 16 GB Mac runs one xcodebuild at a time; a 64 GB
# Mac runs four, so agents in separate worktrees build at once. The classes are
# independent: `xcode` and `swiftpm` holders run together. Go, lint and shell steps need
# no lock.
#
# A slot is a directory (mkdir is atomic): slot 1 is `<lockdir>`, slot k is `<lockdir>.<k>`.
# Waiters queue first-come first-served: each writes a ticket
# `<lockdir>.queue/<epoch-ns>.<pid>` and takes a slot only while its ticket's position
# among the live tickets is no greater than the number of free slots, so no later caller
# passes an earlier one. A ticket or a slot whose pid is gone is stale (`kill -0`) and is
# pruned or taken over (by the oldest waiter only). The holder records `pid` and `cwd` in
# its slot, and a waiter logs its queue position and every holder's pid and cwd every 60 s.
#
# Two xcodebuilds on one `-derivedDataPath` fail with "unable to attach DB: database is
# locked", whatever the slot count. So a command that names `-derivedDataPath <dir>` first
# takes `<dir>.lock` (mkdir, with pid and cwd), and also waits while any xcodebuild outside
# this wrapper still builds into `<dir>`, such as one orphaned by a killed run. Only then
# does it queue for a slot.
#
# Environment:
#   MONACO_XCODE_SLOTS         `xcode` slots (default: the number in the clone's
#                              .git/.monaco/xcode-slots, else max(1, RAM GB / 16))
#   MONACO_SWIFTPM_SLOTS       `swiftpm` slots (default max(1, RAM GB / 8))
#   MONACO_XCODE_LOCK_DIR      `xcode` lock dir (default /private/tmp/monaco-xcodebuild.lock)
#   MONACO_SWIFTPM_LOCK_DIR    `swiftpm` lock dir (default /private/tmp/monaco-swiftpm.lock)
#   MONACO_XCODE_LOCK_TIMEOUT  seconds a waiter waits before exit 75 (default 5400)
#   MONACO_LOCK_HOLD           seconds the command may run before it is stopped and the
#                              script exits 124 (default 1800)
#   MONACO_LOCK_POLL           seconds between checks while waiting (default 2)
#   MONACO_LOCK_WAITED         file to append the whole seconds spent waiting to, once the
#                              lock is taken (`monacoctl agents check` keeps the wait out of
#                              its budget this way)
set -euo pipefail

case "${1:-}" in
  xcode | swiftpm) class="$1"; shift ;;
  *) class=xcode ;;
esac

if [[ $# -eq 0 ]]; then
  echo "usage: $0 [xcode|swiftpm] <command> [args...]" >&2
  exit 2
fi

ram_gb=$(( $(sysctl -n hw.memsize 2>/dev/null || echo 0) / 1073741824 ))
case "$class" in
  xcode)
    base_dir="${MONACO_XCODE_LOCK_DIR:-/private/tmp/monaco-xcodebuild.lock}"
    slots_file="$(git rev-parse --path-format=absolute --git-common-dir 2>/dev/null || true)/.monaco/xcode-slots"
    machine_slots="$(cat "$slots_file" 2>/dev/null || true)"
    slots="${MONACO_XCODE_SLOTS:-${machine_slots:-$((ram_gb / 16))}}"
    ;;
  swiftpm)
    base_dir="${MONACO_SWIFTPM_LOCK_DIR:-/private/tmp/monaco-swiftpm.lock}"
    slots="${MONACO_SWIFTPM_SLOTS:-$((ram_gb / 8))}"
    ;;
esac
if ! [[ "$slots" =~ ^[0-9]+$ ]] || (( slots < 1 )); then
  slots=1
fi
queue_dir="$base_dir.queue"
lock_dir=""     # the slot this process holds, once it holds one
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
have_derived=0  # 1 once this process holds $derived_lock
derived=""      # the command's -derivedDataPath, if it names one
prev=""
for arg in "$@"; do
  if [[ "$prev" == "-derivedDataPath" ]]; then
    derived="${arg%/}"
  fi
  prev="$arg"
done
derived_lock="${derived:+$derived.lock}"
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
  if (( have_derived )); then
    rm -rf "$derived_lock"
    have_derived=0
  fi
}

slot_dir() {
  if (( $1 == 1 )); then
    echo "$base_dir"
  else
    echo "$base_dir.$1"
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

trap release EXIT
# INT and TERM. While waiting in the queue, just exit. While the command runs, pass the
# signal on and let run_capped wait for the command to exit, so the lock is never released
# under a live build. (Inline, because ShellCheck 0.11 flags a function used only by a trap.)
trap 'sig_name=INT; sig_rc=130; if (( running )); then if [[ -n "$child" ]]; then kill -s INT "$child" 2>/dev/null || true; fi; else exit 130; fi' INT
trap 'sig_name=TERM; sig_rc=143; if (( running )); then if [[ -n "$child" ]]; then kill -s TERM "$child" 2>/dev/null || true; fi; else exit 143; fi' TERM

start=$SECONDS
contended=0     # 1 once a wait loop ran; a free lock records 0, since $SECONDS can gain a second in a few ms

# Live xcodebuilds building into $derived, one pid per line. Waiting xcode-lock.sh lines
# are left out: they name xcodebuild only as an argument.
outside_builds() {
  local pid cmd
  ps -axww -o pid,command | while read -r pid cmd; do
    if [[ "$cmd" == *xcodebuild* && "$cmd" != *xcode-lock* ]] &&
      [[ " $cmd " == *" -derivedDataPath $derived "* || " $cmd " == *" -derivedDataPath $derived/ "* ]]; then
      echo "$pid"
    fi
  done
}

if [[ -n "$derived" ]]; then
  mkdir -p "${derived_lock%/*}"
  next_log=0
  while true; do
    owner="$(cat "$derived_lock/pid" 2>/dev/null || true)"
    if [[ -n "$owner" ]] && ! kill -0 "$owner" 2>/dev/null; then
      say "taking over stale $derived_lock from pid $owner"
      rm -rf "$derived_lock"
    fi
    builds="$(outside_builds)"
    if [[ -z "$builds" ]] && mkdir "$derived_lock" 2>/dev/null; then
      have_derived=1
      printf '%s\n' "$PWD" > "$derived_lock/cwd"
      echo "$$" > "$derived_lock/pid"
      break
    fi
    if [[ -d "$derived_lock" ]]; then
      blocker="pid ${owner:-unknown} ($(cat "$derived_lock/cwd" 2>/dev/null || echo unknown))"
    else
      blocker="pid ${builds//[[:space:]]/ } (an xcodebuild outside this lock)"
    fi
    contended=1
    waited=$((SECONDS - start))
    if (( waited >= wait_limit )); then
      say "gave up after ${wait_limit}s waiting for $derived behind $blocker"
      release
      exit 75
    fi
    if (( waited >= next_log )); then
      say "waiting for $derived behind $blocker (${waited}s)"
      next_log=$((waited-waited%60+60))
    fi
    sleep "$poll"
  done
fi

mkdir -p "$queue_dir"
ticket="$(now_ns).$$"
: > "$queue_dir/$ticket"

next_log=0
while true; do
  tickets="$(live_tickets)"
  position="$(awk -v t="$ticket" '$0 == t { print NR }' <<< "$tickets")"
  free=0
  holders=""
  k=0
  while (( k < slots )); do
    k=$((k + 1))
    dir="$(slot_dir "$k")"
    if [[ ! -d "$dir" ]]; then
      free=$((free + 1))
      continue
    fi
    owner="$(cat "$dir/pid" 2>/dev/null || true)"
    if [[ -n "$owner" ]] && ! kill -0 "$owner" 2>/dev/null && [[ "$position" == 1 ]]; then
      say "taking over stale lock from pid $owner"
      rm -rf "$dir"
      free=$((free + 1))
      continue
    fi
    holders="${holders:+$holders, }pid ${owner:-unknown} ($(cat "$dir/cwd" 2>/dev/null || echo unknown))"
  done
  if [[ -n "$position" ]] && (( position <= free )); then
    k=0
    while (( k < slots )); do
      k=$((k + 1))
      dir="$(slot_dir "$k")"
      if mkdir "$dir" 2>/dev/null; then
        lock_dir="$dir"
        have_lock=1
        break 2
      fi
    done
  fi
  contended=1
  waited=$((SECONDS - start))
  if (( waited >= wait_limit )); then
    say "gave up after ${wait_limit}s waiting behind ${holders:-no holder}"
    release
    exit 75
  fi
  if (( waited >= next_log )); then
    say "queue position ${position:-?} behind ${holders:-pid unknown (unknown)} (${waited}s)"
    next_log=$((waited-waited%60+60))
  fi
  sleep "$poll"
done

rm -f "$queue_dir/$ticket"
ticket=""
if [[ -n "${MONACO_LOCK_WAITED:-}" ]]; then
  echo "$(( contended ? SECONDS - start : 0 ))" >> "$MONACO_LOCK_WAITED"
fi
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
