-- name: SwapByID :one
SELECT * FROM swap_views WHERE id = @id;

-- name: SwapBySignature :one
SELECT * FROM swap_views WHERE tx_signature = @tx_signature::text;

-- name: LatestBySource :one
SELECT * FROM swap_views
WHERE source_kind = @source_kind AND source_id = @source_id
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: HasLiveSwap :one
SELECT EXISTS (
  SELECT 1 FROM swaps WHERE source_kind = @source_kind AND source_id = @source_id AND status <> 'failed'
);

-- name: OwnsSignature :one
SELECT EXISTS (SELECT 1 FROM swaps WHERE tx_signature = @tx_signature::text);
