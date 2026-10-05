-- name: ListUserTxns :many
WITH ledger AS (
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
),
ledger_page AS (
  SELECT p.id, p.kind, p.status, p.cabal_id, p.tx_signature, p.created_at, sum(e.amount)::bigint AS amount
  FROM ledger AS p
  JOIN user_txn_entries AS e ON e.txn_id = p.id AND e.account = 'wallet' AND e.asset = @usdc_asset
  GROUP BY p.id, p.kind, p.status, p.cabal_id, p.tx_signature, p.created_at
),
open_funds AS (
  SELECT f.id, 'fund'::text AS kind,
    CASE WHEN f.status = 'failed' THEN 'failed' ELSE 'pending' END::text AS status,
    f.cabal_id, f.tx_signature, f.created_at, (-f.amount_micros)::bigint AS amount
  FROM fund_transfers AS f
  WHERE f.user_id = @user_id AND f.status <> 'settled'
    AND (NOT @has_cursor::bool OR (f.created_at, f.id) < (@cursor_at::timestamptz, @cursor_id::uuid))
  ORDER BY f.created_at DESC, f.id DESC
  LIMIT @row_limit
)
SELECT u.id, u.kind, u.status, u.cabal_id, u.tx_signature, u.created_at, u.amount
FROM (
  SELECT l.id, l.kind, l.status, l.cabal_id, l.tx_signature, l.created_at, l.amount FROM ledger_page AS l
  UNION ALL
  SELECT o.id, o.kind, o.status, o.cabal_id, o.tx_signature, o.created_at, o.amount FROM open_funds AS o
) AS u
ORDER BY u.created_at DESC, u.id DESC
LIMIT @row_limit;
