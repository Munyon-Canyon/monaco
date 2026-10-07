-- name: InsertReport :one
INSERT INTO reports (id, reporter_id, kind, target_id, reason, note, created_at)
VALUES ($1, $2, $3, $4, $5, NULLIF(sqlc.arg(note)::text, ''), $6)
ON CONFLICT (reporter_id, kind, target_id) WHERE status = 'open' DO NOTHING
RETURNING id;

-- name: OpenReportID :one
SELECT id FROM reports
WHERE reporter_id = $1 AND kind = $2 AND target_id = $3 AND status = 'open';

-- name: ListReports :many
SELECT id, reporter_id, kind, target_id, reason, note, status, created_at FROM reports
WHERE status = sqlc.arg(status)::text
  AND (
    NOT sqlc.arg(has_cursor)::boolean
    OR (created_at, id) > (sqlc.arg(after_at)::timestamptz, sqlc.arg(after_id)::uuid)
  )
ORDER BY created_at, id
LIMIT sqlc.arg(row_limit)::int;
