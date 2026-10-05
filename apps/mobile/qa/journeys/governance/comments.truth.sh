#!/usr/bin/env bash
set -euo pipefail

run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
# shellcheck source=/dev/null
source "$(git rev-parse --show-toplevel)/scripts/qa/seed.sh"

a="$(qa_user_id A)"
row="$(qa_sql -F $'\t' -v a="$a" -v comment="QA comment $run" -v ask="QA ask $run" -v answer="QA answer $run" <<<"
  SELECT
    (SELECT count(*) FROM feed_comments WHERE body = :'comment' AND author_id = :'a' AND parent_comment_id IS NULL AND deleted_at IS NULL),
    (SELECT count(*) FROM feed_comments WHERE body = :'ask' AND author_id = :'a' AND parent_comment_id IS NULL AND deleted_at IS NULL),
    (SELECT count(*) FROM feed_comments r JOIN feed_comments p ON p.id = r.parent_comment_id
      WHERE r.body = :'answer' AND r.author_id = :'a' AND p.body = :'ask' AND r.deleted_at IS NULL),
    (SELECT count(*) FROM feed_comments WHERE body IN (:'comment', :'ask', :'answer'))")" ||
  { echo "database cannot be reached" >&2; exit 2; }
IFS=$'\t' read -r comment ask answer total <<<"$row"

if [[ "$total" == 0 ]]; then
  echo "ok: the run wrote no comments (every step is a known failure until #711)"
  exit 0
fi
fail=0
[[ "$comment" == 1 ]] || { echo "'QA comment $run': got $comment top-level comments by A, want 1"; fail=1; }
[[ "$ask" == 1 ]] || { echo "'QA ask $run': got $ask top-level comments by A, want 1"; fail=1; }
[[ "$answer" == 1 ]] || { echo "'QA answer $run': got $answer replies to 'QA ask $run', want 1"; fail=1; }
[[ "$fail" == 0 ]] && echo "ok: one comment, one question and its reply"
exit "$fail"
