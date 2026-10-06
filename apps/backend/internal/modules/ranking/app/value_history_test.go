package app_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type fakeSnapshots struct {
	rows         []domain.Snapshot
	err          error
	since, until time.Time
}

func (f *fakeSnapshots) SnapshotsSince(
	_ context.Context, _ ids.CabalID, since, until time.Time,
) ([]domain.Snapshot, error) {
	f.since, f.until = since, until
	return f.rows, f.err
}

type fakeContributions struct {
	points []treasury.ContributionPoint
	err    error
}

func (f fakeContributions) CabalContributionHistory(
	context.Context,
	ids.CabalID,
) ([]treasury.ContributionPoint, error) {
	return f.points, f.err
}

type countingLoader struct{ loads int }

func (c *countingLoader) Load(
	ctx context.Context, _ app.HistoryKey, load func(context.Context) (domain.ValueHistory, error),
) (domain.ValueHistory, error) {
	c.loads++
	return load(ctx)
}

func historyFixture() (app.ReadValueHistory, domain.Run, *fakeSnapshots) {
	at := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	run := domain.Run{ID: ids.Real{}.NewV7(), Rev: 1, AsOf: at, PricesAsOf: at.Add(-time.Minute)}
	snaps := &fakeSnapshots{rows: []domain.Snapshot{{At: at, Value: money.MicrosFromUint64(500)}}}
	in := app.ReadValueHistory{
		Cabal: ids.CabalIDFrom(ids.Real{}.NewV7()), Range: domain.Range1H,
		Check: func(context.Context, ids.CabalID) error { return nil },
	}
	return in, run, snaps
}

func TestReadValueHistory_BeforeTheFirstRunIsEmptyButChecksTheCabal(t *testing.T) {
	t.Parallel()
	in, _, snaps := historyFixture()
	got, err := in.Run(t.Context(), fakeBoards{}, snaps, fakeContributions{})
	if err != nil || got.Points == nil || len(got.Points) != 0 || got.PricesAsOf != nil {
		t.Fatalf("Run = %+v, %v, want empty points and no prices_as_of", got, err)
	}
	in.Check = func(context.Context, ids.CabalID) error { return errs.New(errs.CodeCabalNotFound, "test") }
	if _, err := in.Run(
		t.Context(),
		fakeBoards{},
		snaps,
		fakeContributions{},
	); errs.CodeOf(
		err,
	) != errs.CodeCabalNotFound {
		t.Fatalf("err = %v, want cabal_not_found", err)
	}
}

func TestReadValueHistory_ReadsFromTheRunsAsOfAndLoadsOncePerKey(t *testing.T) {
	t.Parallel()
	in, run, snaps := historyFixture()
	loader := &countingLoader{}
	in.Histories = loader
	contributions := fakeContributions{points: []treasury.ContributionPoint{
		{At: run.AsOf, NetContributed: money.SignedMicrosFromInt64(200)},
	}}
	got, err := in.Run(t.Context(), fakeBoards{run: run, hasRun: true}, snaps, contributions)
	if err != nil || loader.loads != 1 || len(got.Points) != 1 || got.Points[0].PnL.Int64() != 300 ||
		got.PricesAsOf == nil || !got.PricesAsOf.Equal(run.PricesAsOf) {
		t.Fatalf("Run = %+v, %v, loads %d", got, err, loader.loads)
	}
	if !snaps.until.Equal(run.AsOf) || !snaps.since.Equal(run.AsOf.Add(-time.Hour)) {
		t.Fatalf("read %v..%v, want the hour up to the run's as_of", snaps.since, snaps.until)
	}
	in.Range, in.Histories = domain.RangeAll, nil
	if _, err = in.Run(t.Context(), fakeBoards{run: run, hasRun: true}, snaps, contributions); err != nil ||
		!snaps.since.Equal(time.Unix(0, 0)) {
		t.Fatalf("ALL since = %v, %v, want the epoch", snaps.since, err)
	}
}

func TestReadValueHistory_Failures(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeInternal, "test")
	for name, tc := range map[string]struct {
		boards fakeBoards
		check  app.CabalCheck
		snaps  error
		ledger error
		rows   []domain.Snapshot
		want   errs.Code
	}{
		"latest run": {boards: fakeBoards{failing: "run"}, want: errs.CodeInternal},
		"cabal": {
			check: func(context.Context, ids.CabalID) error { return errs.New(errs.CodeCabalNotFound, "test") },
			want:  errs.CodeCabalNotFound,
		},
		"snapshots": {snaps: boom, want: errs.CodeInternal},
		"ledger":    {ledger: boom, want: errs.CodeInternal},
		"curve": {
			rows: []domain.Snapshot{{
				At:    time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC),
				Value: money.MicrosFromUint64(1 << 62),
			}},
			ledger: nil, want: errs.CodeInternal,
		},
	} {
		in, run, snaps := historyFixture()
		tc.boards.run, tc.boards.hasRun = run, tc.boards.failing == ""
		snaps.err = tc.snaps
		if tc.rows != nil {
			snaps.rows = tc.rows
		}
		if tc.check != nil {
			in.Check = tc.check
		}
		points := []treasury.ContributionPoint{}
		if name == "curve" {
			points = append(
				points,
				treasury.ContributionPoint{At: run.AsOf, NetContributed: money.SignedMicrosFromInt64(math.MinInt64)},
			)
		}
		if _, err := in.Run(
			t.Context(),
			tc.boards,
			snaps,
			fakeContributions{points: points, err: tc.ledger},
		); errs.CodeOf(
			err,
		) != tc.want {
			t.Errorf("%s: err = %v, want %s", name, err, tc.want)
		}
	}
}
