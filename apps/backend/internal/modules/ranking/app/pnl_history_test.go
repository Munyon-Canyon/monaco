package app_test

import (
	"context"
	"math"
	"slices"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type fakeStakes struct {
	points []app.StakePoint
	err    error
}

func (f fakeStakes) UserStakeHistory(context.Context, ids.UserID) ([]app.StakePoint, error) {
	return f.points, f.err
}

type fakeCabalSnapshots struct {
	series  map[ids.CabalID][]domain.Snapshot
	err     error
	cabals  []ids.CabalID
	since   time.Time
	queries int
}

func (f *fakeCabalSnapshots) SnapshotsOfCabals(
	_ context.Context, cabals []ids.CabalID, since, _ time.Time,
) (map[ids.CabalID][]domain.Snapshot, error) {
	f.queries++
	f.cabals, f.since = cabals, since
	return f.series, f.err
}

func TestReadPnLHistory_ReadsEachCabalOnceFromOneSnapshotQuery(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	run := domain.Run{ID: ids.Real{}.NewV7(), AsOf: at}
	a, b := ids.CabalIDFrom(ids.Real{}.NewV7()), ids.CabalIDFrom(ids.Real{}.NewV7())
	point := func(cabal ids.CabalID, shares uint64, net int64) app.StakePoint {
		return app.StakePoint{
			CabalID: cabal, At: at.Add(-time.Hour), ShareUnits: money.SharesUnitsFromUint64(shares),
			NetContributed: money.SignedMicrosFromInt64(net),
		}
	}
	snap := domain.Snapshot{At: at, Value: money.MicrosFromUint64(300), TotalShares: money.SharesUnitsFromUint64(10)}
	snaps := &fakeCabalSnapshots{series: map[ids.CabalID][]domain.Snapshot{a: {snap}, b: {snap}}}
	stakes := fakeStakes{points: []app.StakePoint{point(a, 10, 100), point(b, 5, 50), point(a, 10, 100)}}
	in := app.ReadPnLHistory{User: ids.UserIDFrom(ids.Real{}.NewV7()), Range: domain.Range1H}
	got, err := in.Run(t.Context(), fakeBoards{run: run, hasRun: true}, snaps, stakes)
	if err != nil || len(got) != 1 || got[0].Equity.Uint64() != 450 || got[0].PnL.Int64() != 300 {
		t.Fatalf("Run = %+v, %v, want equity 450 and P&L 300", got, err)
	}
	if snaps.queries != 1 || len(snaps.cabals) != 2 || !snaps.since.Equal(at.Add(-time.Hour)) {
		t.Fatalf("snapshots read %d times for %v since %v", snaps.queries, snaps.cabals, snaps.since)
	}
	in.Range = domain.RangeAll
	if _, err = in.Run(t.Context(), fakeBoards{run: run, hasRun: true}, snaps, stakes); err != nil ||
		!snaps.since.Equal(time.Unix(0, 0)) {
		t.Fatalf("ALL since = %v, %v, want the epoch", snaps.since, err)
	}
}

func TestReadPnLHistory_IsEmptyWithoutARunOrAStakeAndReadsNoSnapshots(t *testing.T) {
	t.Parallel()
	run := domain.Run{ID: ids.Real{}.NewV7()}
	snaps := &fakeCabalSnapshots{}
	in := app.ReadPnLHistory{Range: domain.Range1D}
	for name, boards := range map[string]fakeBoards{"no run": {}, "no stake": {run: run, hasRun: true}} {
		got, err := in.Run(t.Context(), boards, snaps, fakeStakes{})
		if err != nil || got == nil || len(got) != 0 || snaps.queries != 0 {
			t.Fatalf("%s: Run = %#v, %v, snapshot reads %d", name, got, err, snaps.queries)
		}
	}
}

func TestReadPnLHistory_SkipsACabalWhoseStakeOutgrowsItsSnapshotInsteadOfFailing(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	cabal := ids.CabalIDFrom(ids.Real{}.NewV7())
	held := []app.StakePoint{{CabalID: cabal, At: at.Add(-time.Hour), ShareUnits: money.SharesUnitsFromUint64(2)}}
	snaps := &fakeCabalSnapshots{series: map[ids.CabalID][]domain.Snapshot{
		cabal: {{At: at, Value: money.MicrosFromUint64(10), TotalShares: money.SharesUnitsFromUint64(1)}},
	}}
	in := app.ReadPnLHistory{Range: domain.Range1H}
	boards := fakeBoards{run: domain.Run{AsOf: at}, hasRun: true}
	got, err := in.Run(t.Context(), boards, snaps, fakeStakes{points: held})
	if err != nil || len(got) != 0 {
		t.Fatalf("Run = %+v, %v, want no points and no error", got, err)
	}
}

func TestReadPnLHistory_Failures(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeInternal, "test")
	at := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	cabal := ids.CabalIDFrom(ids.Real{}.NewV7())
	held := []app.StakePoint{{CabalID: cabal, At: at, ShareUnits: money.SharesUnitsFromUint64(2)}}
	for name, tc := range map[string]struct {
		boards fakeBoards
		stakes fakeStakes
		snaps  *fakeCabalSnapshots
	}{
		"latest run": {boards: fakeBoards{failing: "run"}, snaps: &fakeCabalSnapshots{}},
		"stakes": {
			boards: fakeBoards{run: domain.Run{AsOf: at}, hasRun: true}, stakes: fakeStakes{err: boom},
			snaps: &fakeCabalSnapshots{},
		},
		"snapshots": {
			boards: fakeBoards{run: domain.Run{AsOf: at}, hasRun: true}, stakes: fakeStakes{points: held},
			snaps: &fakeCabalSnapshots{err: boom},
		},
		"curve": {
			boards: fakeBoards{run: domain.Run{AsOf: at}, hasRun: true}, stakes: fakeStakes{points: held},
			snaps: &fakeCabalSnapshots{series: map[ids.CabalID][]domain.Snapshot{
				cabal: {{At: at, Value: money.MicrosFromUint64(math.MaxUint64), TotalShares: money.SharesUnitsFromUint64(2)}},
			}},
		},
	} {
		in := app.ReadPnLHistory{Range: domain.Range1H}
		if _, err := in.Run(t.Context(), tc.boards, tc.snaps, tc.stakes); errs.CodeOf(err) != errs.CodeInternal {
			t.Errorf("%s: err = %v, want internal", name, err)
		}
	}
}

func hourlySnapshots(now time.Time, at func(hour int) (uint64, uint64)) []domain.Snapshot {
	const hours = 48
	out := make([]domain.Snapshot, 0, hours+1)
	for h := hours; h >= 0; h-- {
		value, total := at(h)
		out = append(out, domain.Snapshot{
			At: now.Add(-time.Duration(h) * time.Hour), Value: money.MicrosFromUint64(value),
			TotalShares: money.SharesUnitsFromUint64(total),
		})
	}
	return out
}

func stake(cabal ids.CabalID, at time.Time, shares uint64, net int64) app.StakePoint {
	return app.StakePoint{
		CabalID: cabal, At: at, ShareUnits: money.SharesUnitsFromUint64(shares),
		NetContributed: money.SignedMicrosFromInt64(net),
	}
}

func TestReadPnLHistory_OneCabalReadsOnlyThatCabalsStake(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	boards := fakeBoards{run: domain.Run{ID: ids.Real{}.NewV7(), AsOf: now}, hasRun: true}
	a, b := ids.CabalIDFrom(ids.Real{}.NewV7()), ids.CabalIDFrom(ids.Real{}.NewV7())
	flat := func(value uint64) []domain.Snapshot {
		return hourlySnapshots(now, func(int) (uint64, uint64) { return value, 10 })
	}
	stakes := fakeStakes{points: []app.StakePoint{
		stake(a, now.Add(-30*time.Hour), 10, 100), stake(b, now.Add(-29*time.Hour), 5, 50),
	}}
	snaps := &fakeCabalSnapshots{series: map[ids.CabalID][]domain.Snapshot{a: flat(300), b: flat(80)}}
	for cabal, want := range map[ids.CabalID]struct {
		equity uint64
		pnl    int64
	}{a: {300, 200}, b: {40, -10}} {
		in := app.ReadPnLHistory{Range: domain.Range1D, Cabal: &cabal}
		got, err := in.Run(t.Context(), boards, snaps, stakes)
		if err != nil || len(got) == 0 {
			t.Fatalf("Run = %+v, %v", got, err)
		}
		last := got[len(got)-1]
		if last.Equity.Uint64() != want.equity || last.PnL.Int64() != want.pnl {
			t.Fatalf("last = equity %d, P&L %d, want %v", last.Equity.Uint64(), last.PnL.Int64(), want)
		}
		if len(snaps.cabals) != 1 || snaps.cabals[0] != cabal {
			t.Fatalf("snapshots read for %v, want only %v", snaps.cabals, cabal)
		}
	}
}

func TestReadPnLHistory_AFundAfterTheFirstBucketRaisesNetContributedNotPnL(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	boards := fakeBoards{run: domain.Run{ID: ids.Real{}.NewV7(), AsOf: now}, hasRun: true}
	cabal := ids.CabalIDFrom(ids.Real{}.NewV7())
	series := hourlySnapshots(now, func(h int) (uint64, uint64) {
		if h >= 6 {
			return 300, 10
		}
		return 450, 15
	})
	stakes := fakeStakes{points: []app.StakePoint{
		stake(cabal, now.Add(-30*time.Hour), 10, 100), stake(cabal, now.Add(-6*time.Hour), 15, 250),
	}}
	snaps := &fakeCabalSnapshots{series: map[ids.CabalID][]domain.Snapshot{cabal: series}}
	in := app.ReadPnLHistory{Range: domain.Range1D, Cabal: &cabal}
	got, err := in.Run(t.Context(), boards, snaps, stakes)
	if err != nil || len(got) < 2 {
		t.Fatalf("Run = %+v, %v", got, err)
	}
	for i, p := range got {
		if p.PnL.Int64() != 200 {
			t.Fatalf("point %d P&L = %d, want 200 across the fund", i, p.PnL.Int64())
		}
	}
	if first, last := got[0].Equity.Uint64(), got[len(got)-1].Equity.Uint64(); first != 300 || last != 450 {
		t.Fatalf("equity %d to %d, want 300 to 450", first, last)
	}
}

func TestReadPnLHistory_ACashOutAndRefundKeepsTheRealisedPnL(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	boards := fakeBoards{run: domain.Run{ID: ids.Real{}.NewV7(), AsOf: now}, hasRun: true}
	cabal := ids.CabalIDFrom(ids.Real{}.NewV7())
	series := hourlySnapshots(now, func(int) (uint64, uint64) { return 300, 10 })
	stakes := fakeStakes{points: []app.StakePoint{
		stake(cabal, now.Add(-30*time.Hour), 10, 100),
		stake(cabal, now.Add(-20*time.Hour), 0, -200),
		stake(cabal, now.Add(-10*time.Hour), 10, -100),
	}}
	snaps := &fakeCabalSnapshots{series: map[ids.CabalID][]domain.Snapshot{cabal: series}}
	filtered, err := app.ReadPnLHistory{Range: domain.Range1D, Cabal: &cabal}.Run(t.Context(), boards, snaps, stakes)
	if err != nil || len(filtered) == 0 {
		t.Fatalf("Run = %+v, %v", filtered, err)
	}
	if last := filtered[len(filtered)-1]; last.Equity.Uint64() != 300 || last.PnL.Int64() != 400 {
		t.Fatalf("last = %+v, want equity 300 and P&L 400", last)
	}
	summed, err := app.ReadPnLHistory{Range: domain.Range1D}.Run(t.Context(), boards, snaps, stakes)
	if err != nil || !slices.Equal(summed, filtered) {
		t.Fatalf("summed = %+v, %v, want the filtered curve %+v", summed, err, filtered)
	}
}

func TestReadPnLHistory_ACabalNeverHeldIsEmptyAndReadsNoSnapshots(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	boards := fakeBoards{run: domain.Run{ID: ids.Real{}.NewV7(), AsOf: now}, hasRun: true}
	held, other := ids.CabalIDFrom(ids.Real{}.NewV7()), ids.CabalIDFrom(ids.Real{}.NewV7())
	snaps := &fakeCabalSnapshots{}
	stakes := fakeStakes{points: []app.StakePoint{stake(held, now.Add(-time.Hour), 10, 100)}}
	got, err := app.ReadPnLHistory{Range: domain.Range1D, Cabal: &other}.Run(t.Context(), boards, snaps, stakes)
	if err != nil || got == nil || len(got) != 0 || snaps.queries != 0 {
		t.Fatalf("Run = %#v, %v, snapshot reads %d, want empty points and no read", got, err, snaps.queries)
	}
}

func TestReadPnLHistory_PerCabalHistoriesSumToTheUnfilteredHistory(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	boards := fakeBoards{run: domain.Run{ID: ids.Real{}.NewV7(), AsOf: now}, hasRun: true}
	rapid.Check(t, func(t *rapid.T) {
		count := rapid.IntRange(1, 3).Draw(t, "cabals")
		var history []app.StakePoint
		series := map[ids.CabalID][]domain.Snapshot{}
		cabals := make([]ids.CabalID, count)
		for i := range cabals {
			cabal := ids.CabalIDFrom(ids.Real{}.NewV7())
			cabals[i] = cabal
			total := rapid.Uint64Range(10, 20).Draw(t, "total")
			value := rapid.Uint64Range(0, 1_000_000).Draw(t, "value")
			series[cabal] = hourlySnapshots(now, func(h int) (uint64, uint64) {
				if h%2 == 0 {
					return value, total
				}
				return value + 1000, total
			})
			for j, n := 0, rapid.IntRange(1, 3).Draw(t, "stakes"); j < n; j++ {
				at := now.Add(-time.Duration(47-j*3-i) * time.Hour)
				history = append(history, stake(
					cabal, at, rapid.Uint64Range(0, total).Draw(t, "shares"),
					rapid.Int64Range(-1000, 5000).Draw(t, "net"),
				))
			}
		}
		slices.SortStableFunc(history, func(x, y app.StakePoint) int { return x.At.Compare(y.At) })
		snaps := &fakeCabalSnapshots{series: series}
		stakes := fakeStakes{points: history}
		all, err := app.ReadPnLHistory{Range: domain.Range1D}.Run(t.Context(), boards, snaps, stakes)
		if err != nil {
			t.Fatal(err)
		}
		assertSumsTo(t, boards, snaps, stakes, cabals, all)
	})
}

func assertSumsTo(
	t *rapid.T, boards fakeBoards, snaps *fakeCabalSnapshots, stakes fakeStakes, cabals []ids.CabalID,
	all []domain.PnLPoint,
) {
	t.Helper()
	for i, p := range all {
		var equity uint64
		var pnl int64
		for _, cabal := range cabals {
			one, err := app.ReadPnLHistory{Range: domain.Range1D, Cabal: &cabal}.Run(t.Context(), boards, snaps, stakes)
			if err != nil {
				t.Fatal(err)
			}
			at := slices.IndexFunc(one, func(q domain.PnLPoint) bool { return q.At.Equal(p.At) })
			if at < 0 {
				t.Fatalf("cabal %v has no point at %v", cabal, p.At)
			}
			equity += one[at].Equity.Uint64()
			pnl += one[at].PnL.Int64()
		}
		if equity != p.Equity.Uint64() || pnl != p.PnL.Int64() {
			t.Fatalf("bucket %d: per-cabal sum = %d / %d, unfiltered = %d / %d",
				i, equity, pnl, p.Equity.Uint64(), p.PnL.Int64())
		}
	}
}

func TestReadPnLHistory_ThreeAndSixMonthsStartThatFarBackWithinTheMonthlyBudget(t *testing.T) {
	t.Parallel()
	const days = 200
	at := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	cabal := ids.CabalIDFrom(ids.Real{}.NewV7())
	stakes := fakeStakes{points: []app.StakePoint{{
		CabalID: cabal, At: at.AddDate(0, 0, -days-1), ShareUnits: money.SharesUnitsFromUint64(1),
		NetContributed: money.SignedMicrosFromInt64(1000),
	}}}
	series := make([]domain.Snapshot, 0, days+1)
	for d := days; d >= 0; d-- {
		series = append(series, domain.Snapshot{
			At: at.AddDate(0, 0, -d), Value: money.MicrosFromUint64(uint64(1000 + days - d)),
			TotalShares: money.SharesUnitsFromUint64(1),
		})
	}
	snaps := &fakeCabalSnapshots{series: map[ids.CabalID][]domain.Snapshot{cabal: series}}
	boards := fakeBoards{run: domain.Run{AsOf: at}, hasRun: true}
	read := func(r domain.Range) []domain.PnLPoint {
		got, err := app.ReadPnLHistory{Range: r}.Run(t.Context(), boards, snaps, stakes)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	month := len(read(domain.Range1M))
	for r, span := range map[domain.Range]int{domain.Range3M: 90, domain.Range6M: 182} {
		got := read(r)
		if len(got) == 0 || len(got) > month || !got[0].At.Equal(at.AddDate(0, 0, -span)) ||
			!got[len(got)-1].At.Equal(at) {
			t.Fatalf("%s: %d points from %v to %v, want at most %d from %d days back to now",
				r, len(got), got[0].At, got[len(got)-1].At, month, span)
		}
	}
}
