package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const seedPrunable = `WITH e AS (
  INSERT INTO events (id, aggregate_type, aggregate_id, type, payload, actor_type, actor_id)
  VALUES (gen_random_uuid(), 'cabal', gen_random_uuid(), 'cabal.created', '{"v":1}', 'system', 'test') RETURNING id
)
INSERT INTO event_deliveries (handler, event_id, code, handled_at)
SELECT 'h' || n, e.id, 'ok', $2::timestamptz FROM e, generate_series(1, $1::int) AS n`

func TestPruneDeliveries_aFullLastBatchTakesOneMoreEmptyBatchToFinish(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	cutoff := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(t.Context(), seedPrunable, 1000, cutoff.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	deleted, batches, err := db.PruneDeliveries(t.Context(), pool, cutoff)
	if err != nil || deleted != 1000 || batches != 2 {
		t.Fatalf("PruneDeliveries = %d rows in %d batches (%v), want 1000 in 2", deleted, batches, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := db.PruneDeliveries(ctx, pool, cutoff); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("PruneDeliveries with a cancelled context = %v, want db_unavailable", err)
	}
}
