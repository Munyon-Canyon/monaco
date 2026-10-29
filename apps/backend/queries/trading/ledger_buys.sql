-- name: FilledBuysBetween :many
SELECT cabal_id, symbol, confirmed_at
FROM swap_views
WHERE cabal_id = ANY(@cabal_ids::uuid[])
  AND action = 'buy'
  AND status = 'confirmed'
  AND confirmed_at >= @window_start::timestamptz
  AND confirmed_at < @window_end::timestamptz
ORDER BY confirmed_at, id;
