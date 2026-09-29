package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
)

const pruneBatch = 1000

func PruneDeliveries(ctx context.Context, pool *pgxpool.Pool, before time.Time) (deleted, batches int, err error) {
	q := sqlc.New(pool)
	for {
		n, err := q.DeleteDeliveriesBefore(ctx, sqlc.DeleteDeliveriesBeforeParams{Before: before, Batch: pruneBatch})
		if err != nil {
			return deleted, batches, classify(err, "db.PruneDeliveries")
		}
		deleted += int(n)
		batches++
		if n < pruneBatch {
			return deleted, batches, nil
		}
	}
}

func PruneIdempotencyKeys(ctx context.Context, pool *pgxpool.Pool, before time.Time) (int, error) {
	n, err := sqlc.New(pool).DeleteIdempotencyKeysBefore(ctx, before)
	if err != nil {
		return 0, classify(err, "db.PruneIdempotencyKeys")
	}
	return int(n), nil
}
