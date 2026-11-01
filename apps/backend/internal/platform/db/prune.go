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
	return pruneBatches(func() (int64, error) {
		return q.DeleteDeliveriesBefore(ctx, sqlc.DeleteDeliveriesBeforeParams{Before: before, Batch: pruneBatch})
	}, "db.PruneDeliveries")
}

func PruneIdempotencyKeys(ctx context.Context, pool *pgxpool.Pool, before time.Time) (int, error) {
	q := sqlc.New(pool)
	deleted, _, err := pruneBatches(func() (int64, error) {
		return q.DeleteIdempotencyKeysBefore(
			ctx,
			sqlc.DeleteIdempotencyKeysBeforeParams{Before: before, Batch: pruneBatch},
		)
	}, "db.PruneIdempotencyKeys")
	return deleted, err
}

func PruneRateLimitBuckets(ctx context.Context, pool *pgxpool.Pool, idleBefore time.Time) (int, error) {
	q := sqlc.New(pool)
	deleted, _, err := pruneBatches(func() (int64, error) {
		return q.DeleteRateLimitBucketsIdleBefore(ctx, sqlc.DeleteRateLimitBucketsIdleBeforeParams{
			Before: idleBefore, Batch: pruneBatch,
		})
	}, "db.PruneRateLimitBuckets")
	return deleted, err
}

func pruneBatches(next func() (int64, error), op string) (deleted, batches int, err error) {
	for {
		n, err := next()
		if err != nil {
			return deleted, batches, classify(err, op)
		}
		deleted += int(n)
		batches++
		if n < pruneBatch {
			return deleted, batches, nil
		}
	}
}
