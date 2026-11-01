-- name: ListStuckTreasuryMoney :many
SELECT kind::text AS kind, ref::text AS ref, user_id::uuid AS user_id, cabal_id::uuid AS cabal_id,
  status::text AS status, since::timestamptz AS since
FROM (
  SELECT 'fund_transfer' AS kind, id::text AS ref, user_id, cabal_id, status, created_at AS since
  FROM fund_transfers WHERE status IN ('created', 'submitted', 'landed')
  UNION ALL
  SELECT 'cash_out_job', id::text, user_id, cabal_id, status, updated_at
  FROM cash_out_jobs WHERE status IN ('started', 'selling', 'paying')
  UNION ALL
  SELECT 'cash_out_payout', p.job_id::text || ':' || p.attempt::text, j.user_id, j.cabal_id, p.status, p.created_at
  FROM cash_out_payouts p JOIN cash_out_jobs j ON j.id = p.job_id WHERE p.status IN ('signed', 'broadcast')
  UNION ALL
  SELECT 'wind_down', cabal_id::text, '00000000-0000-0000-0000-000000000000'::uuid, cabal_id, status, started_at
  FROM cabal_winddowns WHERE status = 'running'
) m
WHERE since < sqlc.arg(created_before)::timestamptz AND (sqlc.arg(kind)::text = '' OR kind = sqlc.arg(kind)::text)
ORDER BY since, ref
LIMIT sqlc.arg(row_limit)::bigint;
