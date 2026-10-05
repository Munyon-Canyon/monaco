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
