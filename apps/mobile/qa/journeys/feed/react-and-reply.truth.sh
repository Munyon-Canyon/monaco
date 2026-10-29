#!/usr/bin/env bash
set -euo pipefail

run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
# shellcheck source=/dev/null
source "$(git rev-parse --show-toplevel)/scripts/qa/seed.sh"

a="$(qa_user_id A)"
row="$(qa_sql -F $'\t' -v a="$a" -v run="$run" <<<"
  WITH sale AS (SELECT id FROM feed_objects WHERE cabal_name = 'QA feed ' || :'run' AND kind = 'trade'),
  pub AS (SELECT o.id FROM feed_objects o JOIN cabals c ON c.id = o.cabal_id WHERE c.name = 'QA public ' || :'run' AND o.kind = 'proposal'),
  root AS (SELECT id FROM feed_comments WHERE body = 'QA root ' || :'run')
  SELECT
    (SELECT count(*) FROM sale),
    (SELECT count(*) FROM feed_reactions r JOIN sale ON sale.id = r.feed_object_id WHERE r.user_id = :'a' AND r.emoji = U&'\\+01F525'),
    (SELECT count(*) FROM feed_comments c JOIN sale ON sale.id = c.feed_object_id WHERE c.author_id = :'a' AND c.body = 'QA feed reply ' || :'run' AND c.deleted_at IS NOT NULL),
    (SELECT count(*) FROM feed_comments c JOIN comment_attachments t ON t.comment_id = c.id AND t.kind = 'gif' JOIN root ON root.id = c.parent_comment_id
      WHERE c.author_id = :'a' AND c.deleted_at IS NULL),
    (SELECT count(*) FROM comment_reactions r JOIN root ON root.id = r.comment_id WHERE r.user_id = :'a' AND r.emoji = U&'\\+01F389'),
    (SELECT count(*) FROM comment_reactions r JOIN feed_comments c ON c.id = r.comment_id JOIN pub ON pub.id = c.feed_object_id
      WHERE r.user_id = :'a' AND r.emoji = U&'\\+01F44D'),
    (SELECT count(*) FROM feed_comments c JOIN pub ON pub.id = c.feed_object_id WHERE c.author_id = :'a')")" ||
  { echo "database cannot be reached" >&2; exit 2; }
IFS=$'\t' read -r sale fire text gif party thumb publicreply <<<"$row"

if [[ "$sale" == 0 ]]; then
  echo "ok: the run seeded nothing"
  exit 0
fi
fail=0
[[ "$fire" == 1 ]] || { echo "A's 🔥 on the sale: got $fire, want 1"; fail=1; }
[[ "$text" == 1 ]] || { echo "A's text reply 'QA feed reply $run' deleted: got $text, want 1"; fail=1; }
[[ "$gif" == 1 ]] || { echo "A's live GIF replies to 'QA root $run': got $gif, want 1"; fail=1; }
[[ "$party" == 1 ]] || { echo "A's 🎉 on 'QA root $run': got $party, want 1"; fail=1; }
[[ "$thumb" == 1 ]] || { echo "A's 👍 on the public reply: got $thumb, want 1"; fail=1; }
[[ "$publicreply" == 0 ]] || { echo "A's replies on the public proposal: got $publicreply, want 0"; fail=1; }
[[ "$fail" == 0 ]] && echo "ok: A's reaction, GIF reply, deleted reply, two emoji chips and no public reply"
exit "$fail"
