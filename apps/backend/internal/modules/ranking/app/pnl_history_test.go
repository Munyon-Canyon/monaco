package app_test

import (
	"context"
	"math"
	"testing"
	"time"

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
