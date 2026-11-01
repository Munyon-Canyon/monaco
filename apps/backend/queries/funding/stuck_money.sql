-- name: ListStuckFundingMoney :many
SELECT kind::text AS kind, ref::text AS ref, user_id::uuid AS user_id, cabal_id::uuid AS cabal_id,
  status::text AS status, since::timestamptz AS since
FROM (
  SELECT 'withdrawal' AS kind, id::text AS ref, user_id, '00000000-0000-0000-0000-000000000000'::uuid AS cabal_id,
    status, created_at AS since
  FROM withdrawals WHERE status IN ('created', 'submitted')
  UNION ALL
  SELECT 'evm_forward', id::text, user_id, '00000000-0000-0000-0000-000000000000'::uuid, status, created_at
  FROM evm_deposit_forwards WHERE status IN ('pending', 'failed')
  UNION ALL
  SELECT 'external_deposit', id::text, '00000000-0000-0000-0000-000000000000'::uuid, cabal_id, status, detected_at
  FROM external_deposits WHERE status IN ('detected', 'bouncing', 'bounce_failed')
  UNION ALL
  SELECT 'onramp_session', id::text, user_id, coalesce(cabal_id, '00000000-0000-0000-0000-000000000000'::uuid),
    status, created_at
  FROM onramp_sessions
  WHERE status IN ('confirmed', 'submitted') OR (status IN ('created', 'opened') AND expires_at < sqlc.arg(now)::timestamptz)
  UNION ALL
  SELECT 'deposit_candidate', tx_signature || ':' || wallet_address, user_id,
    '00000000-0000-0000-0000-000000000000'::uuid, status, seen_at
  FROM deposit_candidates WHERE status = 'pending'
) m
WHERE since < sqlc.arg(created_before)::timestamptz AND (sqlc.arg(kind)::text = '' OR kind = sqlc.arg(kind)::text)
ORDER BY since, ref
LIMIT sqlc.arg(row_limit)::bigint;
