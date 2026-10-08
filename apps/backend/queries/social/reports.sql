-- name: InsertReport :one
INSERT INTO reports (id, reporter_id, kind, target_id, reason, note, created_at)
VALUES ($1, $2, $3, $4, $5, NULLIF(sqlc.arg(note)::text, ''), $6)
ON CONFLICT (reporter_id, kind, target_id) WHERE status = 'open' DO NOTHING
RETURNING id;

-- name: OpenReportID :one
SELECT id FROM reports
WHERE reporter_id = $1 AND kind = $2 AND target_id = $3 AND status = 'open';
