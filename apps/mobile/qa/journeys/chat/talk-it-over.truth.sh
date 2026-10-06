#!/usr/bin/env bash
set -euo pipefail

run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
# shellcheck source=/dev/null
source "$(git rev-parse --show-toplevel)/scripts/qa/seed.sh"

name="QA $run"
a="$(_qa_account A privy_user_id)"
b="$(_qa_account B privy_user_id)"
rows="$(qa_sql -F '|' -v name="$name" -v run="$run" -v a="$a" -v b="$b" <<<"SELECT
  coalesce(string_agg(m.body || ':' || (u.privy_user_id = :'a')::int || ':' || (u.privy_user_id = :'b')::int || ':' ||
    (m.deleted_at IS NOT NULL)::int || ':' || m.reply_count || ':' || m.also_in_channel::int || ':' || (m.parent_id IS NOT NULL)::int,
    ' ' ORDER BY m.created_at, m.id), '')
  FROM cabal_messages m JOIN cabals c ON c.id = m.cabal_id JOIN users u ON u.id = m.author_id WHERE c.name = :'name'")" ||
  { echo "database cannot be reached" >&2; exit 2; }
if [[ -z "$rows" ]]; then
  echo "ok: the run never created a chat message in $name"
  exit 0
fi
want="gm $run:1:0:0:1:0:0 gm back:0:1:0:0:0:1 typo $run:1:0:1:0:0:0 one $run:1:0:0:0:0:0 two $run:1:0:0:0:0:0"
if [[ "$rows" == "$want" ]]; then
  echo "ok: $name has gm with one reply, gm back by B outside the channel, typo deleted, one and two live"
else
  echo "$name messages (body:by A:by B:deleted:replies:in channel:is reply): got '$rows', want '$want'"
  exit 1
fi
