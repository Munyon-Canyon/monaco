-- name: QAPotDestination :one
SELECT address FROM user_wallets WHERE user_id = $1;
