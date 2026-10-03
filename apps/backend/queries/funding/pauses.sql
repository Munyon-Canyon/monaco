-- name: LockPauseScope :exec
SELECT pg_advisory_xact_lock(hashtextextended(
  'cabal-pause:' || coalesce(NULLIF(sqlc.arg(cabal_id)::uuid, '00000000-0000-0000-0000-000000000000')::text, 'global'),
  0));

-- name: InsertPause :exec
INSERT INTO cabal_pauses (id, cabal_id, reason, note, created_by, created_at)
VALUES (sqlc.arg(id)::uuid, NULLIF(sqlc.arg(cabal_id)::uuid, '00000000-0000-0000-0000-000000000000'),
  sqlc.arg(reason)::text, sqlc.arg(note)::text,
  NULLIF(sqlc.arg(created_by)::uuid, '00000000-0000-0000-0000-000000000000'), sqlc.arg(created_at)::timestamptz);

-- name: OpenScopePauses :many
SELECT reason FROM cabal_pauses
WHERE resolved_at IS NULL
  AND coalesce(cabal_id, '00000000-0000-0000-0000-000000000000') = sqlc.arg(cabal_id)::uuid
ORDER BY created_at, id;

-- name: ResolveOpsPauses :execrows
UPDATE cabal_pauses
SET resolved_at = sqlc.arg(resolved_at)::timestamptz,
  resolved_by = NULLIF(sqlc.arg(resolved_by)::uuid, '00000000-0000-0000-0000-000000000000')
WHERE resolved_at IS NULL AND reason = 'ops'
  AND coalesce(cabal_id, '00000000-0000-0000-0000-000000000000') = sqlc.arg(cabal_id)::uuid;

-- name: PauseScope :one
SELECT coalesce(cabal_id, '00000000-0000-0000-0000-000000000000')::uuid AS cabal_id
FROM cabal_pauses WHERE id = $1;

-- name: ResolvePause :many
UPDATE cabal_pauses SET resolved_at = sqlc.arg(resolved_at)::timestamptz
WHERE id = $1 AND resolved_at IS NULL
RETURNING reason;
