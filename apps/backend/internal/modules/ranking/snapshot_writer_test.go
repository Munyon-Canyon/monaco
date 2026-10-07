package ranking_test

import (
	"maps"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestSnapshotWriterWritesTheRunAndSnapshot(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	cabal := testkit.NewCabal(t, pool)
	writer := app.NewSnapshotWriter(
		db.New(pool, testkit.NewIDs(586), testkit.NewClock(now)),
		testkit.NewIDs(587),
	)
	valuation := app.Valuation{
		AsOf: now, PricesAsOf: now,
		Cabals: []app.CabalValue{{
			CabalID: cabal.ID, Value: money.MicrosFromUint64(2_000_000),
			NavPerShare: money.MicrosFromUint64(2_000_000), TotalShares: money.SharesUnitsFromUint64(1_000_000),
		}},
		Entries:  []app.Entry{entry(now, 1, 1)},
		Excluded: 1,
	}
	runID, err := writer.Write(
		observability.WithActor(t.Context(), "system:ranking.valuation"),
		valuation,
		now,
		now.Add(time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	var value, perShare, shares int64
	if err := pool.QueryRow(t.Context(), `SELECT value_micros, nav_per_share_micros, total_shares
		FROM cabal_value_snapshots WHERE cabal_id = $1`, cabal.ID.UUID()).Scan(&value, &perShare, &shares); err != nil {
		t.Fatal(err)
	}
	if value != 2_000_000 || perShare != 2_000_000 || shares != 1_000_000 {
		t.Fatalf("snapshot = (%d, %d, %d)", value, perShare, shares)
	}
	var rows, excluded int
	if err := pool.QueryRow(t.Context(), `SELECT rows_written, cabals_excluded FROM leaderboard_runs WHERE run_id = $1`, runID).
		Scan(&rows, &excluded); err != nil {
		t.Fatal(err)
	}
	if rows != 1 || excluded != 1 {
		t.Fatalf("run = (%d, %d)", rows, excluded)
	}
}

func TestSnapshotWriterWritesOneSnapshotPerCabalInOneStatement(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	valuation := app.Valuation{AsOf: now, PricesAsOf: now, Entries: []app.Entry{entry(now, 1, 1)}}
	want := map[uuid.UUID]int64{}
	for i := range 3 {
		id := ids.CabalIDFrom(ids.Real{}.NewV7())
		valuation.Cabals = append(valuation.Cabals, app.CabalValue{
			CabalID: id, Value: money.MicrosFromUint64(uint64(100 + i)),
			NavPerShare: money.MicrosFromUint64(2), TotalShares: money.SharesUnitsFromUint64(50),
		})
		want[id.UUID()] = int64(100 + i)
	}
	if _, err := newWriter(pool, now).Write(
		observability.WithActor(t.Context(), "system:ranking.valuation"), valuation, now, now.Add(time.Second),
	); err != nil {
		t.Fatal(err)
	}
	rows, err := pool.Query(t.Context(), `SELECT cabal_id, value_micros FROM cabal_value_snapshots WHERE at = $1`, now)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[uuid.UUID]int64{}
	for rows.Next() {
		var id uuid.UUID
		var value int64
		if err := rows.Scan(&id, &value); err != nil {
			t.Fatal(err)
		}
		got[id] = value
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(got, want) {
		t.Fatalf("snapshots = %v, want %v", got, want)
	}
}

func newWriter(pool *pgxpool.Pool, now time.Time) app.SnapshotWriter {
	return app.NewSnapshotWriter(db.New(pool, testkit.NewIDs(586), testkit.NewClock(now)), testkit.NewIDs(587))
}

func entry(now time.Time, rank int, value int64) app.Entry {
	return app.Entry{
		Board: "people", Range: "ALL", Rank: rank, SubjectID: ids.Real{}.NewV7(), SubjectName: "n",
		SubjectCreatedAt: now, ValueMicros: value, PricesAsOf: now, ComputedAt: now, Flags: []string{},
	}
}

func TestSnapshotWriterReplacesTheAllRowsAndCountsThem(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	ctx := observability.WithActor(t.Context(), "system:ranking.valuation")
	writer := newWriter(pool, now)
	if _, err := writer.Write(ctx, app.Valuation{AsOf: now, PricesAsOf: now, Entries: []app.Entry{entry(now, 1, 7)}},
		now, now); err != nil {
		t.Fatal(err)
	}
	later := now.Add(time.Minute)
	runID, err := writer.Write(ctx, app.Valuation{
		AsOf: later, PricesAsOf: later,
		Entries: []app.Entry{entry(later, 1, 9), entry(later, 2, 8)},
	}, later, later)
	if err != nil {
		t.Fatal(err)
	}
	if n := count(t, pool, `SELECT count(*) FROM leaderboard_entries WHERE range = 'ALL'`); n != 2 {
		t.Fatalf("ALL rows = %d, want the second run's 2", n)
	}
	if n := count(t, pool, `SELECT rows_written FROM leaderboard_runs ORDER BY finished_at DESC LIMIT 1`); n != 2 {
		t.Fatalf("rows_written = %d, want 2", n)
	}
	var rows int
	if err := pool.QueryRow(
		t.Context(),
		`SELECT (payload->>'rows_written')::int FROM events WHERE type = 'ranking.snapshot_written' AND aggregate_id = $1`,
		runID,
	).Scan(&rows); err != nil || rows != 2 {
		t.Fatalf("event rows_written = %d, %v", rows, err)
	}
}

func TestSnapshotWriterRollsBackTheDeleteWhenTheInsertFails(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	ctx := observability.WithActor(t.Context(), "system:ranking.valuation")
	writer := newWriter(pool, now)
	cabal := testkit.NewCabal(t, pool)
	old := app.Valuation{AsOf: now, PricesAsOf: now, Entries: []app.Entry{entry(now, 1, 7)}}
	if _, err := writer.Write(ctx, old, now, now); err != nil {
		t.Fatal(err)
	}
	runs := count(t, pool, `SELECT count(*) FROM leaderboard_runs`)
	events := count(t, pool, `SELECT count(*) FROM events WHERE type = 'ranking.snapshot_written'`)
	later := now.Add(time.Minute)
	broken := app.Valuation{
		AsOf: later, PricesAsOf: later,
		Cabals: []app.CabalValue{{
			CabalID: cabal.ID, Value: money.MicrosFromUint64(1), NavPerShare: money.MicrosFromUint64(1),
			TotalShares: money.SharesUnitsFromUint64(1),
		}},
		Entries: []app.Entry{entry(later, 1, 1), entry(later, 1, 2)},
	}
	if _, err := writer.Write(ctx, broken, later, later); err == nil {
		t.Fatal("Write() with a duplicate (board, range, rank) = nil, want the insert to fail")
	}
	var value int64
	if err := pool.QueryRow(t.Context(), `SELECT value_micros FROM leaderboard_entries WHERE range = 'ALL'`).
		Scan(&value); err != nil ||
		value != 7 {
		t.Fatalf("old ALL row = (%d, %v), want the delete rolled back and the old row of 7 kept", value, err)
	}
	if n := count(t, pool, `SELECT count(*) FROM leaderboard_runs`); n != runs {
		t.Fatalf("runs = %d, want %d", n, runs)
	}
	if n := count(t, pool, `SELECT count(*) FROM events WHERE type = 'ranking.snapshot_written'`); n != events {
		t.Fatalf("events = %d, want %d", n, events)
	}
	if n := count(t, pool, `SELECT count(*) FROM cabal_value_snapshots`); n != 0 {
		t.Fatalf("snapshots = %d, want 0", n)
	}
}

func TestSnapshotWriterSecondWriteAtTheSameInstantMovesNothing(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	ctx := observability.WithActor(t.Context(), "system:ranking.valuation")
	writer := newWriter(pool, now)
	cabal := testkit.NewCabal(t, pool)
	valuation := app.Valuation{
		AsOf: now, PricesAsOf: now,
		Cabals: []app.CabalValue{{
			CabalID: cabal.ID, Value: money.MicrosFromUint64(1), NavPerShare: money.MicrosFromUint64(1),
			TotalShares: money.SharesUnitsFromUint64(1),
		}},
		Entries: []app.Entry{entry(now, 1, 7)},
	}
	if _, err := writer.Write(ctx, valuation, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(ctx, valuation, now, now); err == nil {
		t.Fatal("second Write() at the same as_of = nil, want the snapshot key to refuse it")
	}
	for query, want := range map[string]int{
		`SELECT count(*) FROM leaderboard_runs`:                                     1,
		`SELECT count(*) FROM cabal_value_snapshots`:                                1,
		`SELECT count(*) FROM events WHERE type = 'ranking.snapshot_written'`:       1,
		`SELECT count(*) FROM leaderboard_entries WHERE range = 'ALL' AND rank = 1`: 1,
	} {
		if n := count(t, pool, query); n != want {
			t.Fatalf("%s = %d, want %d", query, n, want)
		}
	}
}

func TestSnapshotWriterRefusesAnUnbuiltValuationAndKeepsTheOldRows(t *testing.T) {
	t.Parallel()
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	later := now.Add(time.Minute)
	cabal := app.CabalValue{
		CabalID: ids.CabalIDFrom(ids.Real{}.NewV7()), Value: money.MicrosFromUint64(1),
		NavPerShare: money.MicrosFromUint64(1), TotalShares: money.SharesUnitsFromUint64(1),
	}
	for name, valuation := range map[string]app.Valuation{
		"entries not built":          {AsOf: later, PricesAsOf: later},
		"valued cabals with no rows": {AsOf: later, PricesAsOf: later, Cabals: []app.CabalValue{cabal}, Entries: []app.Entry{}},
		"run-shaped valuation":       {AsOf: later, PricesAsOf: later, Cabals: []app.CabalValue{cabal}, Excluded: 1},
		"every cabal excluded":       {AsOf: later, PricesAsOf: later, Entries: []app.Entry{}, Excluded: 2},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assertRefusedKeepingOldRows(t, valuation, now)
		})
	}
}

func assertRefusedKeepingOldRows(t *testing.T, valuation app.Valuation, now time.Time) {
	t.Helper()
	pool := testkit.DB(t)
	ctx := observability.WithActor(t.Context(), "system:ranking.valuation")
	writer := newWriter(pool, now)
	old := app.Valuation{AsOf: now, PricesAsOf: now, Entries: []app.Entry{entry(now, 1, 7)}}
	if _, err := writer.Write(ctx, old, now, now); err != nil {
		t.Fatal(err)
	}
	runs := count(t, pool, `SELECT count(*) FROM leaderboard_runs`)
	events := count(t, pool, `SELECT count(*) FROM events WHERE type = 'ranking.snapshot_written'`)
	if _, err := writer.Write(ctx, valuation, now.Add(time.Minute), now.Add(time.Minute)); err == nil {
		t.Fatal("Write() of an unbuilt valuation = nil, want a refusal")
	}
	for query, want := range map[string]int{
		`SELECT value_micros FROM leaderboard_entries WHERE range = 'ALL'`:    7,
		`SELECT count(*) FROM leaderboard_runs`:                               runs,
		`SELECT count(*) FROM events WHERE type = 'ranking.snapshot_written'`: events,
	} {
		if n := count(t, pool, query); n != want {
			t.Fatalf("%s = %d, want %d", query, n, want)
		}
	}
}

func BenchmarkWriteValuation_500Cabals(b *testing.B) {
	pool := testkit.DB(b)
	start := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	valuation := app.Valuation{PricesAsOf: start, Entries: make([]app.Entry, 0, 500)}
	for i := range 500 {
		valuation.Cabals = append(valuation.Cabals, app.CabalValue{
			CabalID: ids.CabalIDFrom(ids.Real{}.NewV7()), Value: money.MicrosFromUint64(1_000_000),
			NavPerShare: money.MicrosFromUint64(1_000_000), TotalShares: money.SharesUnitsFromUint64(1),
		})
		valuation.Entries = append(valuation.Entries, entry(start, i+1, 1_000_000))
	}
	writer := newWriter(pool, start)
	ctx := observability.WithActor(b.Context(), "system:ranking.valuation")
	var n int
	b.ResetTimer()
	for b.Loop() {
		n++
		valuation.AsOf = start.Add(time.Duration(n) * time.Minute)
		if _, err := writer.Write(ctx, valuation, valuation.AsOf, valuation.AsOf.Add(time.Second)); err != nil {
			b.Fatal(err)
		}
	}
}
