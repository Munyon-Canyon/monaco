#!/bin/bash
set -euo pipefail
APP_ID="$1"; KEY="$2"
b64() { openssl base64 -A | tr '+/' '-_' | tr -d '='; }
now=$(date +%s)
h=$(printf '{"alg":"RS256","typ":"JWT"}' | b64)
p=$(printf '{"iat":%d,"exp":%d,"iss":"%s"}' $((now-60)) $((now+540)) "$APP_ID" | b64)
s=$(printf '%s.%s' "$h" "$p" | openssl dgst -sha256 -sign "$KEY" | b64)
printf '%s.%s.%s' "$h" "$p" "$s"
