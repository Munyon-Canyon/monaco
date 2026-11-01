-- name: ActorByPrivyUserID :one
SELECT u.id, u.account_status,
  (EXISTS (SELECT 1 FROM revoked_sessions r WHERE r.privy_session_id = sqlc.arg(privy_session_id)::text)
    OR COALESCE(date_trunc('second', u.sessions_valid_after) >= sqlc.arg(issued_at)::timestamptz, false))::boolean
    AS session_ended
FROM users u
WHERE u.privy_user_id = sqlc.arg(privy_user_id)::text;

-- name: ActorByID :one
SELECT id, account_status FROM users WHERE id = $1;

-- name: RecordUserSession :execrows
INSERT INTO user_sessions (privy_session_id, user_id, first_seen_at)
VALUES (sqlc.arg(privy_session_id)::text, sqlc.arg(user_id)::uuid, sqlc.arg(first_seen_at)::timestamptz)
ON CONFLICT (privy_session_id) DO NOTHING;

-- name: RevokeKnownSessions :execrows
INSERT INTO revoked_sessions (privy_session_id, user_id, revoked_at)
SELECT s.privy_session_id, s.user_id, sqlc.arg(revoked_at)::timestamptz
FROM user_sessions s
WHERE s.user_id = sqlc.arg(user_id)::uuid
ON CONFLICT (privy_session_id) DO NOTHING;
