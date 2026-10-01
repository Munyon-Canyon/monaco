#!/usr/bin/env bash
# Print one job-summary markdown block: step times, the build timing top 15,
# the 10 slowest tests, and (when FAIL_LOGS names them) the last 60 raw lines
# of each failed step.
set -euo pipefail

times="${STEP_TIMES:-}"
build_log="${BUILD_LOG:-}"
test_log="${TEST_LOG:-}"
fail_logs="${FAIL_LOGS:-}"

echo "### Step times"
echo
if [[ -n "$times" ]]; then
  if [[ -s "$times" ]]; then
    echo '```'
    printf '%s\n' "step	seconds"
    cat "$times"
    echo '```'
  else
    echo "None."
  fi
else
  echo "None."
fi
echo

echo "### Build Timing Summary"
echo
if [[ -n "$build_log" ]]; then
  if [[ -f "$build_log" ]]; then
    timing="$(
      set +o pipefail
      awk '
        { sub(/^[0-9][0-9]:[0-9][0-9]:[0-9][0-9] /, "") }
        /Build Timing Summary/ { capture=1; pending=""; next }
        !capture { next }
        /seconds/ {
          line=$0
          gsub(/^[ \t]+|[ \t]+$/, "", line)
          if (line ~ /^[0-9]+(\.[0-9]+)? seconds/) {
            if (pending != "") {
              line=pending " / " line
            }
          }
          if (match(line, /[0-9]+(\.[0-9]+)? seconds/)) {
            num=substr(line, RSTART, RLENGTH)
            sub(/ seconds/, "", num)
            printf "%s\t%s\n", num, line
          }
          pending=""
          next
        }
        {
          gsub(/^[ \t]+|[ \t]+$/, "", $0)
          if ($0 != "") pending=$0
        }
      ' "$build_log" | sort -t "$(printf '\t')" -k1,1gr | awk -F '\t' 'NR<=15 { print $2 }'
    )"
    if [[ -n "$timing" ]]; then
      echo '```'
      printf '%s\n' "$timing"
      echo '```'
    else
      echo "None."
    fi
  else
    echo "None."
  fi
else
  echo "None."
fi
echo

echo "### Slowest tests"
echo
if [[ -n "$test_log" ]]; then
  if [[ -f "$test_log" ]]; then
    slow="$(
      set +o pipefail
      awk '
        { sub(/^[0-9][0-9]:[0-9][0-9]:[0-9][0-9] /, "") }
        function emit(name, secs) {
          gsub(/^[ \t]+|[ \t]+$/, "", name)
          gsub(/✔/, "", name)
          gsub(/◇/, "", name)
          gsub(/^[ \t]+|[ \t]+$/, "", name)
          if (name == "") return
          if (name ~ /^(Suite|Test run)/) return
          if (name ~ /^run /) return
          sub(/^Test /, "", name)
          printf "%s\t%s\n", secs, name
        }
        {
          if (!match($0, /[0-9]+(\.[0-9]+)? seconds/)) next
          secs=substr($0, RSTART, RLENGTH)
          sub(/ seconds/, "", secs)
          if (index($0, "Test Case ")) {
            if (index($0, " passed (")) {
              name=$0
              sub(/^.*Test Case /, "", name)
              sub(/ passed \(.*/, "", name)
              q=sprintf("%c", 39)
              if (substr(name, 1, 1) == q) name=substr(name, 2)
              n=index(name, q)
              if (n > 0) name=substr(name, 1, n-1)
              emit(name, secs)
              next
            }
          }
          if (index($0, "passed after ")) {
            name=substr($0, 1, index($0, "passed after ")-1)
            emit(name, secs)
            next
          }
          if (index($0, "✔")) {
            name=$0
            sub(/ *\([0-9].*seconds\).*/, "", name)
            emit(name, secs)
            next
          }
          if (index($0, "◇")) {
            name=$0
            sub(/ *\([0-9].*seconds\).*/, "", name)
            emit(name, secs)
          }
        }
      ' "$test_log" | sort -t "$(printf '\t')" -k1,1gr | awk -F '\t' '!seen[$2]++ { out[++n] = $0 } END { i=1; while (i <= n) { if (i <= 10) print out[i]; i=i+1 } }'
    )"
    if [[ -n "$slow" ]]; then
      echo '```'
      printf '%s\n' "seconds	test"
      while IFS=$'\t' read -r secs name; do
        printf '%s\t%s\n' "$secs" "$name"
      done <<< "$slow"
      echo '```'
    else
      echo "None."
    fi
  else
    echo "None."
  fi
else
  echo "None."
fi
echo

if [[ -n "$fail_logs" ]]; then
  if [[ -f "$fail_logs" ]]; then
    while IFS=$'\t' read -r title path; do
      if [[ -z "$title" ]]; then
        continue
      fi
      if [[ ! -f "$path" ]]; then
        continue
      fi
      echo "<details>"
      echo "<summary>Last 60 lines of ${title}</summary>"
      echo
      echo '```'
      tail -n 60 "$path"
      echo '```'
      echo "</details>"
      echo
    done < "$fail_logs"
  fi
fi
