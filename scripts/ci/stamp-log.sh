#!/usr/bin/env bash
# Prefix each stdin line with the local time. The macOS awk on the runner
# (version 20200816) has fflush and no strftime.
set -euo pipefail
exec python3 -u -c 'import datetime, sys
for line in sys.stdin:
    sys.stdout.write(datetime.datetime.now().strftime("%H:%M:%S ") + line)
    sys.stdout.flush()
'
