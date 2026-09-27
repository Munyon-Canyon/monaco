package poller_test

import (
	"context"
	"log/slog"
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

func seed(t *testing.T, pool *pgxpool.Pool, prefix string, n int, at time.Time) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), seedDeliveries, prefix, n, at); err != nil {
		t.Fatal(err)
	}
}

func TestRetention_deletesOnlyDeliveriesOlderThanThirtyDaysInBatches(t *testing.T) {
	t.Parallel()
	pool, clk := testkit.DB(t), testkit.NewClock(epoch())
	if _, err := pool.Exec(t.Context(), seedEvent); err != nil {
		t.Fatal(err)
	}
	seed(t, pool, "old-", 2500, epoch().Add(-31*day))
	seed(t, pool, "edge-", 1, epoch().Add(-30*day))
	seed(t, pool, "new-", 3, epoch().Add(-29*day))
	report, err := poller.NewRetention(pool, clk).Tick(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if report.Scanned != 2500 || report.Changed != 2500 {
		t.Fatalf("report = %+v, want 2500 scanned and deleted", report)
	}
	want := slog.GroupValue(slog.String("table", "event_deliveries"), slog.Time("before", epoch().Add(-30*day)),
		slog.Int("batches", 3))
	if got := slog.GroupValue(report.Attrs...); !got.Equal(want) {
		t.Fatalf("report attrs = %v, want %v", got, want)
	}
	if left := handlers(t, pool); len(left) != 4 || left[0] != "edge-1" || left[3] != "new-3" {
		t.Fatalf("rows left = %v, want the edge row and the three recent rows", left)
	}
}

func handlers(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(t.Context(), `SELECT handler FROM event_deliveries ORDER BY handler`)
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
