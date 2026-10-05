package ranking_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	seedSnapshots = `INSERT INTO cabal_value_snapshots (cabal_id, at, value_micros, nav_per_share_micros, total_shares)
SELECT $1, $2::timestamptz - (n * interval '2 minutes'), n, 1, 1 FROM generate_series(0, $3::int) AS n`
	countSnapshots = `SELECT count(*) FROM cabal_value_snapshots WHERE cabal_id = $1 AND at >= $2`
	countHours     = `SELECT count(DISTINCT date_trunc('hour', at)) FROM cabal_value_snapshots
WHERE cabal_id = $1 AND at < $2`
	countHourlyRows = `SELECT count(*) FROM cabal_value_snapshots WHERE cabal_id = $1 AND at < $2`
	latestHourRows  = `SELECT count(*) FROM cabal_value_snapshots AS s
WHERE s.cabal_id = $1 AND s.at < $2 AND s.at = (SELECT max(o.at) FROM cabal_value_snapshots AS o
  WHERE o.cabal_id = s.cabal_id AND date_trunc('hour', o.at) = date_trunc('hour', s.at) AND o.at < $2)`
	latestAtOrBefore = `SELECT value_micros FROM cabal_value_snapshots WHERE cabal_id = $1 AND at <= $2
ORDER BY at DESC LIMIT 1`
)

func thinningNow() time.Time { return time.Date(2026, 10, 4, 12, 7, 0, 0, time.UTC) }

func seedSeries(t *testing.T, pool *pgxpool.Pool, cabal ids.CabalID, newest time.Time, days int) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), seedSnapshots, cabal.UUID(), newest, days*24*30); err != nil {
		t.Fatal(err)
	}
}

func count(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestThinning_KeepsHourly(t *testing.T) {
	t.Parallel()
	pool, now := testkit.DB(t), thinningNow()
	thin := app.NewThinSnapshots(pool, testkit.NewClock(now))
	old, young := ids.CabalIDFrom(ids.Real{}.NewV7()), ids.CabalIDFrom(ids.Real{}.NewV7())
	seedSeries(t, pool, old, now, 10)
	seedSeries(t, pool, young, now, 3)
	before := now.Add(-app.SnapshotFullRetention)
	recent := count(t, pool, countSnapshots, old.UUID(), before)
	hours := count(t, pool, countHours, old.UUID(), before)
	total := count(t, pool, countHourlyRows, old.UUID(), before)
	report, err := thin.Tick(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got := count(t, pool, countSnapshots, old.UUID(), before); got != recent {
		t.Fatalf("rows in the last 7 days = %d, want all %d", got, recent)
	}
	if got := count(t, pool, countHourlyRows, old.UUID(), before); got != hours {
		t.Fatalf("older rows = %d, want one per hour %d", got, hours)
	}
	if got := count(t, pool, latestHourRows, old.UUID(), before); got != hours {
		t.Fatalf("hours keeping their last row = %d, want %d", got, hours)
	}
	if report.Changed != total-hours || report.Scanned != total-hours {
		t.Fatalf("report = %+v, want %d deleted", report, total-hours)
	}
	if got := count(t, pool, countSnapshots, young.UUID(), now.Add(-4*24*time.Hour)); got != 3*24*30+1 {
		t.Fatalf("young cabal rows = %d, want untouched %d", got, 3*24*30+1)
	}
}

func TestThinning_SecondTickDeletesNothing(t *testing.T) {
	t.Parallel()
	pool, now := testkit.DB(t), thinningNow()
	thin := app.NewThinSnapshots(pool, testkit.NewClock(now))
	cabal := ids.CabalIDFrom(ids.Real{}.NewV7())
	seedSeries(t, pool, cabal, now, 10)
	if _, err := thin.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	report, err := thin.Tick(t.Context())
	if err != nil || report.Changed != 0 || report.Scanned != 0 {
		t.Fatalf("second Tick = %+v, %v, want 0 deleted", report, err)
	}
}

func TestThinning_RangedReadStillFindsARow(t *testing.T) {
	t.Parallel()
	pool, now := testkit.DB(t), thinningNow()
	thin := app.NewThinSnapshots(pool, testkit.NewClock(now))
	cabal := ids.CabalIDFrom(ids.Real{}.NewV7())
	seedSeries(t, pool, cabal, now, 10)
	if _, err := thin.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, ago := range []time.Duration{
		30 * time.Minute, 24 * time.Hour, 7*24*time.Hour + 17*time.Minute, 9*24*time.Hour + 41*time.Minute,
	} {
		var value int64
		if err := pool.QueryRow(t.Context(), latestAtOrBefore, cabal.UUID(), now.Add(-ago)).Scan(&value); err != nil {
			t.Fatalf("ranged read at -%v: %v", ago, err)
		}
	}
}

func TestThinning_TickFailsOnACanceledContext(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := app.NewThinSnapshots(pool, testkit.NewClock(thinningNow())).Tick(ctx); err == nil {
		t.Fatal("Tick error = nil")
	}
}
