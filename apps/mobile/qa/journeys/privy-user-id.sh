#!/usr/bin/env bash
# Prints an actor's Privy user ID from accounts.tsv, or fails naming the actor.
set -euo pipefail

actor="${1:?usage: privy-user-id.sh <actor>}"
id="$(awk -F '\t' -v actor="$actor" '
  $1 == "actor" {
    for (i = 1; i <= NF; i++) {
      if ($i == "actor") actor_column = i
      if ($i == "privy_user_id") privy_user_id_column = i
    }
    next
  }
  actor_column != "" && $actor_column == actor { print $privy_user_id_column; exit }
' "$(dirname "$0")/accounts.tsv")"

if [[ -z "$id" ]]; then
  echo "missing Privy user ID for actor $actor in accounts.tsv" >&2
  exit 1
fi
echo "$id"
