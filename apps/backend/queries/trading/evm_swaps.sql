-- name: MarkSubmittedEVM :execrows
UPDATE swaps
SET status = 'submitted', tx_signature = sqlc.arg(tx_hash)::text, submitted_at = sqlc.arg(submitted_at)::timestamptz,
  updated_at = sqlc.arg(submitted_at)::timestamptz
WHERE id = sqlc.arg(id)::uuid AND status = 'created' AND sqlc.arg(tx_hash)::text <> '';
