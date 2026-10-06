-- name: InsertAdminAction :exec
INSERT INTO admin_actions (id, admin_id, action, target_type, target_id, reason, before, after, approved_by, created_at)
VALUES (
  sqlc.arg(id), sqlc.arg(admin_id), sqlc.arg(action), sqlc.arg(target_type), sqlc.arg(target_id), sqlc.arg(reason),
  sqlc.arg(before), sqlc.arg(after), sqlc.narg(approved_by), sqlc.arg(created_at)
)
ON CONFLICT (id) DO NOTHING;

-- name: ListAdminActions :many
SELECT id, admin_id, action, target_type, target_id, reason, before, after, approved_by, created_at
FROM admin_actions
WHERE (sqlc.narg(admin_id)::uuid IS NULL OR admin_id = sqlc.narg(admin_id)::uuid)
  AND (sqlc.narg(target_type)::text IS NULL OR target_type = sqlc.narg(target_type)::text)
  AND (sqlc.narg(target_id)::text IS NULL OR target_id = sqlc.narg(target_id)::text)
  AND (sqlc.narg(action)::text IS NULL OR action = sqlc.narg(action)::text)
  AND (sqlc.narg(cursor)::uuid IS NULL OR id < sqlc.narg(cursor)::uuid)
ORDER BY id DESC
LIMIT sqlc.arg(row_limit)::bigint;
