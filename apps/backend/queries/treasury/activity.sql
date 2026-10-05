-- name: UpsertActivity :execrows
INSERT INTO cabal_activity AS a (
  id, cabal_id, kind, status, actor_user_id, asset, usdc_micros, units, tx_signature, occurred_at, updated_at
) VALUES (
  sqlc.arg(id)::uuid, sqlc.arg(cabal_id)::uuid, sqlc.arg(kind)::text, sqlc.arg(status)::text,
  sqlc.narg(actor_user_id)::uuid,
  sqlc.narg(asset)::text, sqlc.narg(usdc_micros)::text::numeric,
  sqlc.narg(units)::text::numeric, sqlc.narg(tx_signature)::text, sqlc.arg(at)::timestamptz, sqlc.arg(at)::timestamptz
)
ON CONFLICT (id) DO UPDATE SET
  status = CASE WHEN a.status = 'pending' THEN excluded.status ELSE a.status END,
  actor_user_id = coalesce(a.actor_user_id, excluded.actor_user_id),
  asset = coalesce(a.asset, excluded.asset),
  usdc_micros = coalesce(a.usdc_micros, excluded.usdc_micros),
  units = coalesce(a.units, excluded.units),
  tx_signature = coalesce(a.tx_signature, excluded.tx_signature),
  updated_at = excluded.updated_at
WHERE (
  CASE WHEN a.status = 'pending' THEN excluded.status ELSE a.status END,
  coalesce(a.actor_user_id, excluded.actor_user_id),
  coalesce(a.asset, excluded.asset), coalesce(a.usdc_micros, excluded.usdc_micros),
  coalesce(a.units, excluded.units), coalesce(a.tx_signature, excluded.tx_signature)
) IS DISTINCT FROM (a.status, a.actor_user_id, a.asset, a.usdc_micros, a.units, a.tx_signature);
