-- name: InsertOnrampSession :exec
INSERT INTO onramp_sessions (
  id, user_id, token_hash, suggested_amount_micros, cabal_id, status, created_at, expires_at
) VALUES ($1, $2, $3, NULLIF(sqlc.arg(suggested_amount_micros)::text, '')::numeric, NULLIF(sqlc.arg(cabal_id)::text, '')::uuid,
  'created', $4, $5);

-- name: OpenOnrampSession :one
WITH opened AS (
  UPDATE onramp_sessions SET status = 'opened', opened_at = sqlc.arg(now)::timestamptz
  WHERE onramp_sessions.token_hash = sqlc.arg(token_hash) AND status = 'created'
    AND expires_at > sqlc.arg(now)::timestamptz
  RETURNING id
)
SELECT s.id, s.user_id, s.status, s.expires_at, (s.opened_at IS NOT NULL)::boolean AS was_opened,
  COALESCE(s.suggested_amount_micros::text, '')::text AS suggested_amount_micros,
  (o.id IS NOT NULL)::boolean AS opened_now
FROM onramp_sessions s LEFT JOIN opened o ON o.id = s.id
WHERE s.token_hash = sqlc.arg(token_hash);

-- name: ReportOnrampStatus :one
WITH moved AS (
  UPDATE onramp_sessions SET status = sqlc.arg(to_status)::text,
    provider = COALESCE(NULLIF(sqlc.arg(provider)::text, ''), provider),
    completed_at = sqlc.arg(now)::timestamptz
  WHERE onramp_sessions.id = sqlc.arg(id) AND onramp_sessions.user_id = sqlc.arg(user_id)
    AND onramp_sessions.status = ANY(sqlc.arg(sources)::text[])
  RETURNING id
)
SELECT s.user_id, s.status, s.created_at,
  COALESCE(s.suggested_amount_micros::text, '')::text AS suggested_amount_micros,
  (m.id IS NOT NULL)::boolean AS moved
FROM onramp_sessions s LEFT JOIN moved m ON m.id = s.id
WHERE s.id = sqlc.arg(id);

-- name: GetOnrampSession :one
SELECT id, status, COALESCE(suggested_amount_micros::text, '')::text AS suggested_amount_micros, created_at,
  (completed_at IS NOT NULL)::boolean AS completed, COALESCE(completed_at, created_at)::timestamptz AS completed_at
FROM onramp_sessions
WHERE id = $1 AND user_id = $2;

-- name: ExpireOnrampSessions :many
WITH due AS (
  SELECT id, status FROM onramp_sessions
  WHERE (status = 'created' AND expires_at <= sqlc.arg(now)::timestamptz)
    OR (status = 'opened' AND opened_at <= sqlc.arg(opened_before)::timestamptz)
  ORDER BY created_at
  LIMIT sqlc.arg(batch)
  FOR UPDATE SKIP LOCKED
)
UPDATE onramp_sessions s SET status = 'expired', completed_at = sqlc.arg(now)::timestamptz
FROM due
WHERE s.id = due.id
RETURNING s.id, s.user_id, due.status AS from_status,
  COALESCE(s.suggested_amount_micros::text, '')::text AS suggested_amount_micros;
