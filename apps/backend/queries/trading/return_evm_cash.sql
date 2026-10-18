-- name: CashHolder :one
WITH RECURSIVE chain (id, depth) AS (
  SELECT sqlc.arg(swap_id)::uuid, 0
  UNION ALL
  SELECT r.trade_id, c.depth + 1
  FROM chain AS c
  JOIN trade_steps AS r ON r.step = 'bridge_out' AND r.privy_action_id = 'reused:' || c.id::text
  WHERE c.depth < 32
)
SELECT id::uuid AS holder FROM chain ORDER BY depth DESC LIMIT 1;

-- name: ReturnableCash :one
SELECT s.id, s.cabal_id, s.status, s.parked_at,
  coalesce(o.network, '')::text AS network,
  coalesce(o.status, '')::text AS bridge_out,
  coalesce(o.arrived_micros, w.amount_micros, 0)::text AS landed_micros,
  coalesce(w.status, '')::text AS swap_step,
  EXISTS (SELECT 1 FROM trade_steps AS b WHERE b.trade_id = s.id AND b.step = 'bridge_back')::bool AS returning
FROM swaps AS s
LEFT JOIN trade_steps AS o ON o.trade_id = s.id AND o.step = 'bridge_out'
LEFT JOIN trade_steps AS w ON w.trade_id = s.id AND w.step = 'swap'
WHERE s.id = sqlc.arg(id)::uuid;
