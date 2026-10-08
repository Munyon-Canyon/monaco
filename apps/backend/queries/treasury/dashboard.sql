-- name: LedgerTotals :many
WITH moved AS (
  SELECT t.created_at, t.kind AS metric, abs(e.amount) AS amount
  FROM user_txns AS t
  JOIN user_txn_entries AS e ON e.txn_id = t.id AND e.account = 'wallet' AND e.asset = @usdc::text
  WHERE t.kind IN ('deposit', 'withdrawal', 'fund') AND t.status = 'settled'
    AND t.created_at >= @from_at::timestamptz AND t.created_at < @to_at::timestamptz
  UNION ALL
  SELECT t.created_at, 'cash_out'::text AS metric, abs(e.amount) AS amount
  FROM cabal_txns AS t
  JOIN cabal_txn_entries AS e ON e.txn_id = t.id AND e.account = 'members' AND e.asset = @usdc::text
  WHERE t.kind = 'cash_out' AND t.status = 'settled'
    AND t.created_at >= @from_at::timestamptz AND t.created_at < @to_at::timestamptz
  UNION ALL
  SELECT t.created_at,
    (CASE WHEN e.amount > 0 THEN 'swap_buy' ELSE 'swap_sell' END)::text AS metric, abs(e.amount) AS amount
  FROM cabal_txns AS t
  JOIN cabal_txn_entries AS e ON e.txn_id = t.id AND e.account = 'venue' AND e.asset = @usdc::text
  WHERE t.kind = 'swap' AND t.status = 'settled'
    AND t.created_at >= @from_at::timestamptz AND t.created_at < @to_at::timestamptz
)
SELECT (date_trunc(@bucket::text, moved.created_at AT TIME ZONE 'UTC') AT TIME ZONE 'UTC')::timestamptz AS bucket_start,
  count(*) FILTER (WHERE metric = 'deposit')::bigint AS deposit_count,
  coalesce(sum(amount) FILTER (WHERE metric = 'deposit'), 0)::text AS deposit_micros,
  count(*) FILTER (WHERE metric = 'fund')::bigint AS fund_count,
  coalesce(sum(amount) FILTER (WHERE metric = 'fund'), 0)::text AS fund_micros,
  count(*) FILTER (WHERE metric = 'cash_out')::bigint AS cash_out_count,
  coalesce(sum(amount) FILTER (WHERE metric = 'cash_out'), 0)::text AS cash_out_micros,
  count(*) FILTER (WHERE metric = 'withdrawal')::bigint AS withdrawal_count,
  coalesce(sum(amount) FILTER (WHERE metric = 'withdrawal'), 0)::text AS withdrawal_micros,
  coalesce(sum(amount) FILTER (WHERE metric = 'swap_buy'), 0)::text AS swap_buy_micros,
  coalesce(sum(amount) FILTER (WHERE metric = 'swap_sell'), 0)::text AS swap_sell_micros
FROM moved
GROUP BY 1
ORDER BY 1;

-- name: PlatformBalanceTotal :one
SELECT coalesce(sum(e.amount), 0)::text AS micros
FROM user_txn_entries AS e
JOIN user_txns AS t ON t.id = e.txn_id
WHERE e.account = 'wallet' AND e.asset = @usdc::text AND t.status = 'settled';
