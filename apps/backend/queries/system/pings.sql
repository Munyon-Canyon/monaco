-- name: InsertPing :exec
INSERT INTO system_pings (id, user_id, note)
VALUES ($1, $2, $3)
ON CONFLICT (id) DO NOTHING;

-- name: EchoPing :execrows
UPDATE system_pings SET echoed_at = sqlc.arg(echoed_at)::timestamptz
WHERE id = $1 AND echoed_at IS NULL;

-- name: GetPing :one
SELECT id, note, (echoed_at IS NOT NULL)::boolean AS echoed
FROM system_pings
WHERE id = $1 AND user_id = $2;
