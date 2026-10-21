-- name: ActorByPrivyUserID :one
SELECT u.id, u.account_status,
  (EXISTS (SELECT 1 FROM revoked_sessions r WHERE r.privy_session_id = sqlc.arg(privy_session_id)::text)
    OR COALESCE(date_trunc('second', u.sessions_valid_after) >= sqlc.arg(issued_at)::timestamptz, false))::boolean
    AS session_ended
FROM users u
WHERE u.privy_user_id = sqlc.arg(privy_user_id)::text;

-- name: ActorByID :one
SELECT id, account_status FROM users WHERE id = $1;
