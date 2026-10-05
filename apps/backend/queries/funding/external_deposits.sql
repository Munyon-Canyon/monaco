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
