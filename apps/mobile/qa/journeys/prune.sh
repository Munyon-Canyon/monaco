#!/usr/bin/env bash
# Closes the QA cabals earlier journey runs left behind, so the Cabals tab stays short.
# Only a local database is touched: DATABASE_URL on anything but localhost, 127.0.0.1 or ::1 is
# refused (exit 0, nothing pruned). Only active cabals named `QA %` are considered:
#   - a name whose last word is a run id (6 characters of journey.py's run-id alphabet, such as `QA chat K7M2QX`)
#     is a candidate when that run is not live, at any age. A run is live while it holds
#     $MONACO_QA_LOCK_DIR/monaco-qa-run-<id>.lock (default /tmp), which `/usr/bin/lockf -k -t 0 <lock> true` fails on
#     (`flock -n <lock> true` where there is no lockf, as on Linux; both test the same flock).
#   - any other `QA %` name keeps the age rule: created more than an hour before JOURNEY_RUN_STARTED
#     (ISO time, default now).
# A cabal that holds money is never pruned: any fund_transfers row, or any cabal_positions row
# with units > 0 (a USDC pot or a stock). Otherwise, in one transaction:
#   - a cabal an actor A, B or C created is closed (status banned, as the backend does),
#   - a cabal where an actor is only a member loses that membership (the backend's leave path).
# The run also resets the A, B and C actors' rate-limit buckets so back-to-back journeys don't hit 429s on setup.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"

url="${DATABASE_URL:-}"
if [[ -z "$url" ]]; then
  echo "prune: refused, DATABASE_URL is not set"
  exit 0
fi
authority="${url#*://}"
authority="${authority%%/*}"
hostport="${authority##*@}"
if [[ "$hostport" == \[* ]]; then
  host="${hostport#\[}"
  host="${host%%\]*}"
else
  host="${hostport%%:*}"
fi
case "$host" in
  localhost | 127.0.0.1 | ::1) ;;
  *)
    echo "prune: refused, DATABASE_URL host '$host' is not local, nothing pruned"
    exit 0
    ;;
esac

dids=""
for actor in A B C; do
  did="$("$here/privy-user-id.sh" "$actor" 2> /dev/null || true)"
  [[ -n "$did" ]] && dids="${dids:+$dids,}$did"
done
started="${JOURNEY_RUN_STARTED:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"

# Only the run ids that name an active QA cabal are looked up: every run leaves its lock file behind, and testing
# thousands of them one lockf at a time took 13 s per run (2026-11-04).
ids="$("$here/psql.sh" -v ON_ERROR_STOP=1 -tA << 'SQL'
SELECT DISTINCT right(name, 6) FROM cabals
WHERE status = 'active' AND name LIKE 'QA %' AND name ~ ' [ABCDEFGHJKMNPQRSTUVWXYZ23456789]{6}$';
SQL
)"

live=""
lock_dir="${MONACO_QA_LOCK_DIR:-/tmp}"
for id in $ids; do
  lock="$lock_dir/monaco-qa-run-$id.lock"
  [[ -e "$lock" ]] || continue
  # lockf fails at once when a run holds the lock, so a failure means live. -k keeps the file.
  if [[ -x /usr/bin/lockf ]]; then
    /usr/bin/lockf -k -t 0 "$lock" true 2> /dev/null || live="${live:+$live,}$id"
  else
    flock -n "$lock" true 2> /dev/null || live="${live:+$live,}$id"
  fi
done

report="$("$here/psql.sh" -v ON_ERROR_STOP=1 -tA -F '|' -v "dids={$dids}" -v "started=$started" -v "live={$live}" << 'SQL'
BEGIN;
CREATE TEMP TABLE prune_candidates ON COMMIT DROP AS
SELECT c.id, c.name,
       (c.creator_id = ANY (a.ids)) AS created_by_actor,
       (EXISTS (SELECT 1 FROM fund_transfers f WHERE f.cabal_id = c.id)
        OR EXISTS (SELECT 1 FROM cabal_positions p WHERE p.cabal_id = c.id AND p.units > 0)) AS funded
FROM cabals c, (SELECT ARRAY (SELECT id FROM users WHERE privy_user_id = ANY (:'dids'::text[])) AS ids) a
WHERE c.status = 'active'
  AND c.name LIKE 'QA %'
  AND CASE WHEN c.name ~ ' [ABCDEFGHJKMNPQRSTUVWXYZ23456789]{6}$'
           THEN NOT (right(c.name, 6) = ANY (:'live'::text[]))
           ELSE c.created_at < :'started'::timestamptz - interval '1 hour' END
  AND (c.creator_id = ANY (a.ids)
       OR EXISTS (SELECT 1 FROM cabal_members m
                  WHERE m.cabal_id = c.id AND m.role = 'member' AND m.user_id = ANY (a.ids)));
UPDATE cabals SET status = 'banned', updated_at = now()
WHERE status = 'active' AND id IN (SELECT id FROM prune_candidates WHERE created_by_actor AND NOT funded);
DELETE FROM cabal_members m
USING prune_candidates pc
WHERE m.cabal_id = pc.id AND NOT pc.created_by_actor AND NOT pc.funded AND m.role = 'member'
  AND m.user_id = ANY (ARRAY (SELECT id FROM users WHERE privy_user_id = ANY (:'dids'::text[])));
DELETE FROM rate_limit_buckets
WHERE key LIKE 'op:%:actor:user:%'
  AND split_part(key, ':actor:user:', 2) IN (SELECT id::text FROM users WHERE privy_user_id = ANY (:'dids'::text[]));
SELECT CASE WHEN funded THEN 'skipped' WHEN created_by_actor THEN 'closed' ELSE 'left' END, id, name
FROM prune_candidates ORDER BY name, id;
COMMIT;
SQL
)"

pruned=0
skipped=0
while IFS='|' read -r kind id name; do
  case "$kind" in
    closed | left)
      echo "prune: $kind cabal $id ($name)"
      pruned=$((pruned + 1))
      ;;
    skipped)
      echo "prune: skipped cabal $id ($name): holds funds (fund_transfers or cabal_positions)"
      skipped=$((skipped + 1))
      ;;
  esac
done <<< "$report"
echo "prune: pruned $pruned cabals, skipped $skipped funded"
