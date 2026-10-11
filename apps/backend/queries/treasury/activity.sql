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
  usdc_micros = CASE WHEN a.status = 'pending' THEN coalesce(excluded.usdc_micros, a.usdc_micros)
    ELSE coalesce(a.usdc_micros, excluded.usdc_micros) END,
  units = CASE WHEN a.status = 'pending' THEN coalesce(excluded.units, a.units)
    ELSE coalesce(a.units, excluded.units) END,
  tx_signature = coalesce(a.tx_signature, excluded.tx_signature),
  updated_at = excluded.updated_at
WHERE (
  CASE WHEN a.status = 'pending' THEN excluded.status ELSE a.status END,
  coalesce(a.actor_user_id, excluded.actor_user_id),
  coalesce(a.asset, excluded.asset),
  CASE WHEN a.status = 'pending' THEN coalesce(excluded.usdc_micros, a.usdc_micros)
    ELSE coalesce(a.usdc_micros, excluded.usdc_micros) END,
  CASE WHEN a.status = 'pending' THEN coalesce(excluded.units, a.units)
    ELSE coalesce(a.units, excluded.units) END,
  coalesce(a.tx_signature, excluded.tx_signature)
) IS DISTINCT FROM (a.status, a.actor_user_id, a.asset, a.usdc_micros, a.units, a.tx_signature);
