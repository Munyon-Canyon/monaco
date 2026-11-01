-- name: SummarizeAdminQueues :many
SELECT kind::text AS kind, count(*)::bigint AS total, min(since)::timestamptz AS oldest
FROM (
  SELECT 'dead_letters' AS kind, sqlc.arg(now)::timestamptz AS since FROM dead_letters WHERE status = 'open'
  UNION ALL
  SELECT 'withdrawal', created_at FROM withdrawals WHERE status IN ('created', 'submitted')
  UNION ALL
  SELECT 'evm_forward', created_at FROM evm_deposit_forwards WHERE status IN ('pending', 'failed')
  UNION ALL
  SELECT 'external_deposit', detected_at FROM external_deposits WHERE status IN ('detected', 'bouncing', 'bounce_failed')
  UNION ALL
  SELECT 'onramp_session', created_at FROM onramp_sessions
  WHERE status IN ('confirmed', 'submitted') OR (status IN ('created', 'opened') AND expires_at < sqlc.arg(now)::timestamptz)
  UNION ALL
  SELECT 'deposit_candidate', seen_at FROM deposit_candidates WHERE status = 'pending'
  UNION ALL
  SELECT 'fund_transfer', created_at FROM fund_transfers WHERE status IN ('created', 'submitted', 'landed')
  UNION ALL
  SELECT 'cash_out_job', updated_at FROM cash_out_jobs WHERE status IN ('started', 'selling', 'paying')
  UNION ALL
  SELECT 'cash_out_payout', created_at FROM cash_out_payouts WHERE status IN ('signed', 'broadcast')
  UNION ALL
  SELECT 'wind_down', started_at FROM cabal_winddowns WHERE status = 'running'
) m
WHERE kind = 'dead_letters' OR since < sqlc.arg(created_before)::timestamptz
GROUP BY kind
ORDER BY kind;
