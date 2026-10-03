-- name: ListUserTxns :many
WITH page AS (
  SELECT t.id, t.kind, t.status, t.cabal_id, t.tx_signature, t.created_at
  FROM user_txns AS t
  WHERE t.user_id = @user_id
    AND EXISTS (
      SELECT 1 FROM user_txn_entries AS e
      WHERE e.txn_id = t.id AND e.account = 'wallet' AND e.asset = @usdc_asset
    )
    AND (NOT @has_cursor::bool OR (t.created_at, t.id) < (@cursor_at::timestamptz, @cursor_id::uuid))
  ORDER BY t.created_at DESC, t.id DESC
  LIMIT @row_limit
)
SELECT p.id, p.kind, p.status, p.cabal_id, p.tx_signature, p.created_at, sum(e.amount)::bigint AS amount
FROM page AS p
JOIN user_txn_entries AS e ON e.txn_id = p.id AND e.account = 'wallet' AND e.asset = @usdc_asset
GROUP BY p.id, p.kind, p.status, p.cabal_id, p.tx_signature, p.created_at
ORDER BY p.created_at DESC, p.id DESC;
