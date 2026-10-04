-- name: InsertRankingTrigger :exec
INSERT INTO ranking_triggers (cabal_id, reason, created_at)
VALUES (sqlc.arg(cabal_id), sqlc.arg(reason), sqlc.arg(created_at));

-- name: OldestRankingTrigger :one
SELECT created_at FROM ranking_triggers ORDER BY created_at, id LIMIT 1;

-- name: DeleteRankingTriggersThrough :exec
DELETE FROM ranking_triggers WHERE created_at <= sqlc.arg(started_at);
