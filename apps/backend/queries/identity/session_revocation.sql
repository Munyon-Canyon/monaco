-- name: RevokeSession :execrows
INSERT INTO revoked_sessions (privy_session_id, user_id, revoked_at)
VALUES (sqlc.arg(privy_session_id)::text, sqlc.arg(user_id)::uuid, sqlc.arg(revoked_at)::timestamptz)
ON CONFLICT (privy_session_id) DO NOTHING;

-- name: RevokeSessionsIssuedBefore :execrows
UPDATE users
SET sessions_valid_after = sqlc.arg(valid_after)::timestamptz
WHERE id = sqlc.arg(user_id)::uuid
  AND (sessions_valid_after IS NULL OR sessions_valid_after < sqlc.arg(valid_after)::timestamptz);
