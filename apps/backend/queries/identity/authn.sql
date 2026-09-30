-- name: ActorByPrivyUserID :one
SELECT id, account_status FROM users WHERE privy_user_id = $1;
