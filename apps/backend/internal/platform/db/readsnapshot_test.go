package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestReadSnapshot_runsOneReadOnlyRepeatableReadTransaction(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	var isolation string
	var readOnly string
	err := db.ReadSnapshot(t.Context(), pool, time.Minute, func(ctx context.Context, q sqlc.DBTX) error {
		if err := q.QueryRow(ctx, `SHOW transaction_isolation`).Scan(&isolation); err != nil {
			return err
		}
		return q.QueryRow(ctx, `SHOW transaction_read_only`).Scan(&readOnly)
	})
	if err != nil || isolation != "repeatable read" || readOnly != "on" {
		t.Fatalf("ReadSnapshot() isolation=%q readOnly=%q err=%v", isolation, readOnly, err)
	}
}

func TestReadSnapshot_hidesWhatCommitsAfterTheFirstRead(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	cabal := uuid.New()
	count := `SELECT count(*) FROM cash_out_jobs WHERE cabal_id = $1`
	var first, second int
	err := db.ReadSnapshot(t.Context(), pool, time.Minute, func(ctx context.Context, q sqlc.DBTX) error {
		if err := q.QueryRow(ctx, count, cabal).Scan(&first); err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, `INSERT INTO cash_out_jobs
  (id, cabal_id, user_id, share_units, payout_micros, slice_micros, status, created_at, updated_at)
VALUES ($1, $2, $3, 1, 1, 1, 'started', now(), now())`, uuid.New(), cabal, uuid.New()); err != nil {
			return err
		}
		return q.QueryRow(ctx, count, cabal).Scan(&second)
	})
	if err != nil || first != 0 || second != 0 {
		t.Fatalf("ReadSnapshot() first=%d second=%d err=%v, want both 0", first, second, err)
	}
}

func TestReadSnapshot_returnsTheCallbackError(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	err := db.ReadSnapshot(t.Context(), pool, time.Minute, func(ctx context.Context, q sqlc.DBTX) error {
		_, err := q.Exec(ctx, `INSERT INTO cash_out_jobs (id) VALUES (gen_random_uuid())`)
		return err
	})
	if err == nil {
		t.Fatal("ReadSnapshot() wrote inside a read-only transaction")
	}
}
