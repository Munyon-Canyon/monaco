#!/usr/bin/env bash
# Closes the QA cabals earlier journey runs left behind, so the Cabals tab stays short.
# Only a local database is touched: DATABASE_URL on anything but localhost, 127.0.0.1 or ::1 is
# refused (exit 0, nothing pruned). JOURNEY_RUN_STARTED (ISO time, default now) is when the run began;
# only active cabals named `QA %` created more than an hour before it are considered.
# A cabal that holds money is never pruned: any fund_transfers row, or any cabal_positions row
# with units > 0 (a USDC pot or a stock). Otherwise, in one transaction:
#   - a cabal an actor A, B or C created is closed (status banned, as the backend does),
#   - a cabal where an actor is only a member loses that membership (the backend's leave path).
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

report="$("$here/psql.sh" -v ON_ERROR_STOP=1 -tA -F '|' -v "dids={$dids}" -v "started=$started" << 'SQL'
BEGIN;
CREATE TEMP TABLE prune_candidates ON COMMIT DROP AS
SELECT c.id, c.name,
       (c.creator_id = ANY (a.ids)) AS created_by_actor,
       (EXISTS (SELECT 1 FROM fund_transfers f WHERE f.cabal_id = c.id)
        OR EXISTS (SELECT 1 FROM cabal_positions p WHERE p.cabal_id = c.id AND p.units > 0)) AS funded
FROM cabals c, (SELECT ARRAY (SELECT id FROM users WHERE privy_user_id = ANY (:'dids'::text[])) AS ids) a
WHERE c.status = 'active'
  AND c.name LIKE 'QA %'
  AND c.created_at < :'started'::timestamptz - interval '1 hour'
  AND (c.creator_id = ANY (a.ids)
       OR EXISTS (SELECT 1 FROM cabal_members m
                  WHERE m.cabal_id = c.id AND m.role = 'member' AND m.user_id = ANY (a.ids)));
UPDATE cabals SET status = 'banned', updated_at = now()
WHERE status = 'active' AND id IN (SELECT id FROM prune_candidates WHERE created_by_actor AND NOT funded);
DELETE FROM cabal_members m
USING prune_candidates pc
WHERE m.cabal_id = pc.id AND NOT pc.created_by_actor AND NOT pc.funded AND m.role = 'member'
  AND m.user_id = ANY (ARRAY (SELECT id FROM users WHERE privy_user_id = ANY (:'dids'::text[])));
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
