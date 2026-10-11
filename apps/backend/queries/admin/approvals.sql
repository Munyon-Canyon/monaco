-- name: InsertApproval :execrows
INSERT INTO admin_approvals (id, action, target_id, requested_by, reason, status, created_at, expires_at)
VALUES (
  sqlc.arg(id), sqlc.arg(action), sqlc.arg(target_id), sqlc.arg(requested_by), sqlc.arg(reason), 'pending',
  sqlc.arg(created_at), sqlc.arg(expires_at)
)
ON CONFLICT (action, target_id) WHERE status = 'pending' DO NOTHING;

-- name: GetApproval :one
SELECT id, action, target_id, requested_by, reason, status, decided_by, decided_reason, created_at, decided_at,
  expires_at
FROM admin_approvals
WHERE id = sqlc.arg(id);

-- name: DecideApproval :execrows
UPDATE admin_approvals
SET status = sqlc.arg(to_status), decided_by = sqlc.arg(decided_by)::uuid,
  decided_reason = sqlc.arg(decided_reason)::text, decided_at = sqlc.arg(decided_at)::timestamptz
WHERE id = sqlc.arg(id) AND status = 'pending' AND expires_at > sqlc.arg(decided_at)::timestamptz;

-- name: ExpireApprovals :execrows
UPDATE admin_approvals
SET status = 'expired', decided_at = sqlc.arg(now)::timestamptz
WHERE status = 'pending' AND expires_at <= sqlc.arg(now)::timestamptz;

-- name: ExpirePendingApproval :execrows
UPDATE admin_approvals
SET status = 'expired', decided_at = sqlc.arg(now)::timestamptz
WHERE action = sqlc.arg(action) AND target_id = sqlc.arg(target_id) AND status = 'pending'
  AND expires_at <= sqlc.arg(now)::timestamptz;

-- name: ListApprovals :many
SELECT id, action, target_id, requested_by, reason, status, decided_by, decided_reason, created_at, decided_at,
  expires_at
FROM admin_approvals
WHERE status = sqlc.arg(status)
  AND (sqlc.narg(cursor)::uuid IS NULL OR id < sqlc.narg(cursor)::uuid)
ORDER BY id DESC
LIMIT sqlc.arg(row_limit)::bigint;
