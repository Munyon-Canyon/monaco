-- name: InsertCashoutSellPlan :exec
INSERT INTO cashout_sell_plans (job_id, cabal_id, legs, created_at)
VALUES (@job_id, @cabal_id, @legs::jsonb, @created_at)
ON CONFLICT (job_id) DO NOTHING;

-- name: CashoutSellPlan :one
SELECT legs::text FROM cashout_sell_plans WHERE job_id = @job_id;

-- name: HasSwapForMint :one
SELECT EXISTS (
  SELECT 1 FROM swaps WHERE source_kind = @source_kind AND source_id = @source_id AND in_mint = @in_mint
);
