-- name: LockSponsorScope :exec
SELECT pg_advisory_xact_lock(hashtextextended('sponsor:' || sqlc.arg(scope)::text || ':' || sqlc.arg(scope_key)::text, 0));

-- name: SponsorReservation :one
SELECT lamports::text AS lamports, rent
FROM relayer_sponsor_reservations
WHERE ref = sqlc.arg(ref)::uuid AND scope = 'global' AND released_at IS NULL;

-- name: SponsorSpent :one
SELECT COALESCE(SUM(lamports), 0)::text AS lamports
FROM relayer_sponsor_reservations
WHERE scope = sqlc.arg(scope)::text AND scope_key = sqlc.arg(scope_key)::text AND released_at IS NULL
  AND created_at > sqlc.arg(since)::timestamptz;

-- name: InsertSponsorReservation :exec
INSERT INTO relayer_sponsor_reservations (ref, scope, scope_key, lamports, rent, created_at)
VALUES (sqlc.arg(ref)::uuid, sqlc.arg(scope)::text, sqlc.arg(scope_key)::text, (sqlc.arg(lamports)::text)::numeric,
  sqlc.arg(rent)::boolean, sqlc.arg(created_at)::timestamptz);

-- name: ReleaseSponsorReservation :execrows
UPDATE relayer_sponsor_reservations
SET released_at = sqlc.arg(released_at)::timestamptz
WHERE ref = sqlc.arg(ref)::uuid AND released_at IS NULL;
