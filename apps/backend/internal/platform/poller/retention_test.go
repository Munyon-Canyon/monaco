package poller_test

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	day       = 24 * time.Hour
	seedEvent = `INSERT INTO events (id, aggregate_type, aggregate_id, type, payload, actor_type, actor_id)
VALUES ('00000000-0000-7000-8000-000000000001', 'cabal', gen_random_uuid(), 'cabal.created', '{"v":1}', 'system', 'test')`
	seedDeliveries = `INSERT INTO event_deliveries (handler, event_id, code, handled_at)
SELECT $1 || n, '00000000-0000-7000-8000-000000000001', 'ok', $3::timestamptz FROM generate_series(1, $2::int) AS n`
)

const seedKey = `INSERT INTO idempotency_keys (actor_key, key, request_hash, status, created_at)
VALUES ('user:u1', $1, '\x00', 2, $2)`

func seedKeys(t *testing.T, pool *pgxpool.Pool, at map[string]time.Time) {
	t.Helper()
	for key, created := range at {
		if _, err := pool.Exec(t.Context(), seedKey, key, created); err != nil {
			t.Fatal(err)
		}
	}
}

func seed(t *testing.T, pool *pgxpool.Pool, prefix string, n int, at time.Time) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), seedDeliveries, prefix, n, at); err != nil {
		t.Fatal(err)
	}
}

func TestRetention_deletesDeliveriesOlderThanThirtyDaysAndKeysOlderThanADay(t *testing.T) {
	t.Parallel()
	pool, clk := testkit.DB(t), testkit.NewClock(epoch())
	if _, err := pool.Exec(t.Context(), seedEvent); err != nil {
		t.Fatal(err)
	}
	seed(t, pool, "old-", 2500, epoch().Add(-31*day))
	seed(t, pool, "edge-", 1, epoch().Add(-30*day))
	seed(t, pool, "new-", 3, epoch().Add(-29*day))
	seedKeys(t, pool, map[string]time.Time{
		"stale": epoch().Add(-day - time.Second), "edge": epoch().Add(-day), "fresh": epoch().Add(-time.Hour),
	})
	report, err := poller.NewRetention(pool, clk).Tick(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if report.Scanned != 2501 || report.Changed != 2501 {
		t.Fatalf("report = %+v, want 2501 scanned and deleted", report)
	}
	want := slog.GroupValue(
		slog.GroupAttrs("event_deliveries",
			slog.Int("deleted", 2500), slog.Time("before", epoch().Add(-30*day)), slog.Int("batches", 3)),
		slog.GroupAttrs("idempotency_keys", slog.Int("deleted", 1), slog.Time("before", epoch().Add(-day))))
	if got := slog.GroupValue(report.Attrs...); !got.Equal(want) {
		t.Fatalf("report attrs = %v, want %v", got, want)
	}
	if left := handlers(t, pool); len(left) != 4 || left[0] != "edge-1" || left[3] != "new-3" {
		t.Fatalf("rows left = %v, want the edge row and the three recent rows", left)
	}
	if keys := column(t, pool, `SELECT key FROM idempotency_keys ORDER BY key`); len(keys) != 2 ||
		keys[0] != "edge" || keys[1] != "fresh" {
		t.Fatalf("keys left = %v, want edge and fresh", keys)
	}
}

func handlers(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	return column(t, pool, `SELECT handler FROM event_deliveries ORDER BY handler`)
}

func column(t *testing.T, pool *pgxpool.Pool, query string) []string {
	t.Helper()
	rows, err := pool.Query(t.Context(), query)
	if err != nil {
		t.Fatal(err)
	}
	left, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	return left
}

func TestRetention_reportsACodedErrorWhenTheDeleteFails(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	r := poller.NewRetention(testkit.DB(t), testkit.NewClock(epoch()))
	if r.Name() != "platform.retention" || r.Interval() != day {
		t.Fatalf("retention = %s every %v, want platform.retention every 24h", r.Name(), r.Interval())
	}
	if _, err := r.Tick(ctx); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("Tick with a cancelled context = %v, want db_unavailable", err)
	}
}

func TestRetention_reportsTheKeyPruneFailingAfterTheDeliveriesPrune(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	if _, err := pool.Exec(t.Context(), `DROP TABLE idempotency_keys`); err != nil {
		t.Fatal(err)
	}
	_, err := poller.NewRetention(pool, testkit.NewClock(epoch())).Tick(t.Context())
	if err == nil || !strings.HasPrefix(err.Error(), "db.PruneIdempotencyKeys: ") {
		t.Fatalf("Tick without the idempotency table = %v, want the error from db.PruneIdempotencyKeys", err)
	}
}
