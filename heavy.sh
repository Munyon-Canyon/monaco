#!/bin/bash
# Usage: heavy.sh <command...>
# One heavy job at a time machine-wide, first come first served, run under nice -n 19.
# Waits for the 1-minute load average to drop below the core count before running,
# while holding its place in the queue but not blocking anyone behind it from seeing why.
Q=/private/tmp/claude-501/m7-rest-heavy.queue
mkdir -p "$Q"
CORES=$(sysctl -n hw.ncpu)
ticket="$Q/$(date +%s%N)-$$"
echo "$*" > "$ticket"
trap 'rm -f "$ticket"' EXIT
while :; do
  for t in "$Q"/*; do
    pid=${t##*-}
    kill -0 "$pid" 2>/dev/null || rm -f "$t"
  done
  head=$(ls "$Q" | sort | head -1)
  if [ "$Q/$head" = "$ticket" ]; then
    load=$(sysctl -n vm.loadavg | awk '{print int($2)}')
    freepct=$(memory_pressure 2>/dev/null | awk -F': ' '/free percentage/{gsub("%","",$2);print $2}')
    if [ "$load" -lt "$CORES" ] && [ "${freepct:-100}" -ge 15 ]; then break; fi
    echo "heavy.sh: first in line, waiting for load ($load, need <$CORES) free=${freepct}%" >&2
  else
    echo "heavy.sh: queued behind $(ls "$Q" | sort | awk -v me="${ticket##*/}" '$0<me' | wc -l | tr -d ' ') job(s)" >&2
  fi
  sleep 15
done
echo "heavy.sh: running at $(uptime)" >&2
nice -n 19 "$@"
