-- name: InsertWithdrawalAuthorization :exec
INSERT INTO withdrawal_authorizations (id, user_id, session_id, amount_micros, destination_kind, network, to_address,
  created_at, expires_at)
VALUES ($1, $2, sqlc.arg(session_id)::text, sqlc.arg(amount_micros)::text::numeric, sqlc.arg(destination_kind)::text,
  sqlc.arg(network)::text, sqlc.arg(to_address)::text, sqlc.arg(created_at)::timestamptz,
  sqlc.arg(expires_at)::timestamptz);

-- name: ConsumeWithdrawalAuthorization :execrows
UPDATE withdrawal_authorizations
SET consumed_at = sqlc.arg(consumed_at)::timestamptz, consumed_by = sqlc.arg(consumed_by)::uuid
WHERE id = $1 AND user_id = sqlc.arg(user_id)::uuid AND session_id = sqlc.arg(session_id)::text
  AND consumed_at IS NULL AND expires_at > sqlc.arg(consumed_at)::timestamptz
  AND amount_micros = sqlc.arg(amount_micros)::text::numeric AND destination_kind = sqlc.arg(destination_kind)::text
  AND network = sqlc.arg(network)::text AND to_address = sqlc.arg(to_address)::text;

-- name: GetWithdrawalAuthorization :one
SELECT session_id, amount_micros::text AS amount_micros, destination_kind, network, to_address, expires_at,
  consumed_at IS NOT NULL AS consumed
FROM withdrawal_authorizations
WHERE id = $1 AND user_id = $2;
