-- name: ListActivity :many
SELECT id, kind, status, actor_user_id, asset, usdc_micros, units,
  tx_signature, occurred_at
FROM cabal_activity
WHERE cabal_id = @cabal_id
  AND (NOT @has_cursor::bool OR (occurred_at, id) < (@cursor_at::timestamptz, @cursor_id::uuid))
ORDER BY occurred_at DESC, id DESC
LIMIT @row_limit;
