-- name: InsertExternalDeposit :execrows
INSERT INTO external_deposits (
  id, signature, cabal_id, sender, mint, asset_id, amount, source, status, detected_at, resolved_at
) VALUES (
  sqlc.arg(id)::uuid, sqlc.arg(signature)::text, sqlc.arg(cabal_id)::uuid, sqlc.arg(sender)::text,
  sqlc.arg(mint)::text, NULLIF(sqlc.arg(asset_id)::uuid, '00000000-0000-0000-0000-000000000000'), sqlc.arg(amount)::text::numeric, sqlc.arg(source)::text,
  sqlc.arg(status)::text, sqlc.arg(detected_at)::timestamptz, NULLIF(sqlc.arg(resolved_at)::timestamptz, '0001-01-01 00:00:00+00'::timestamptz)
)
ON CONFLICT (signature) DO NOTHING;

-- name: OwnsBounceSignature :one
SELECT EXISTS (SELECT 1 FROM external_deposits WHERE bounce_signature = sqlc.arg(signature)::text);

-- name: ExternalDepositSeen :one
SELECT EXISTS (SELECT 1 FROM external_deposits WHERE signature = sqlc.arg(signature)::text);

-- name: WatchCursor :one
SELECT coalesce((SELECT last_signature FROM treasury_watch_cursors WHERE cabal_id = sqlc.arg(cabal_id)::uuid), '')::text;

-- name: AdvanceWatchCursor :exec
INSERT INTO treasury_watch_cursors (cabal_id, last_signature, updated_at)
VALUES (sqlc.arg(cabal_id)::uuid, sqlc.arg(last_signature)::text, sqlc.arg(updated_at)::timestamptz)
ON CONFLICT (cabal_id) DO UPDATE SET last_signature = excluded.last_signature, updated_at = excluded.updated_at;
