-- name: NudgeCandidates :many
SELECT id FROM users
WHERE auth_state IN ('AWAITING_PHONE', 'AWAITING_SOCIALS')
  AND account_status = 'active' AND deleted_at IS NULL
  AND auth_state_changed_at <= sqlc.arg(changed_before)::timestamptz
  AND (last_nudged_at IS NULL OR last_nudged_at <= sqlc.arg(nudged_before)::timestamptz)
  AND nudge_count < sqlc.arg(max_nudges)::smallint
  AND id > sqlc.arg(after)::uuid
ORDER BY id
LIMIT sqlc.arg(page);

-- name: MarkNudged :many
UPDATE users SET last_nudged_at = sqlc.arg(now)::timestamptz, nudge_count = nudge_count + 1
WHERE id = ANY(sqlc.arg(ids)::uuid[])
  AND auth_state IN ('AWAITING_PHONE', 'AWAITING_SOCIALS')
  AND account_status = 'active' AND deleted_at IS NULL
  AND auth_state_changed_at <= sqlc.arg(changed_before)::timestamptz
  AND (last_nudged_at IS NULL OR last_nudged_at <= sqlc.arg(nudged_before)::timestamptz)
  AND nudge_count < sqlc.arg(max_nudges)::smallint
RETURNING id, auth_state, nudge_count;
