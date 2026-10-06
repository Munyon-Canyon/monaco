package app

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Snapshots interface {
	SnapshotsSince(ctx context.Context, cabal ids.CabalID, since, until time.Time) ([]domain.Snapshot, error)
}

type ContributionPoint = treasury.ContributionPoint

type Contributions interface {
	CabalContributionHistory(context.Context, ids.CabalID) ([]ContributionPoint, error)
}

type HistoryKey struct {
	RunID uuid.UUID
	Rev   int32
	Cabal ids.CabalID
	Range domain.Range
}

type HistoryLoader interface {
	Load(context.Context, HistoryKey, func(context.Context) (domain.ValueHistory, error)) (domain.ValueHistory, error)
}

type ReadValueHistory struct {
	Cabal     ids.CabalID
	Range     domain.Range
	Check     CabalCheck
	Histories HistoryLoader
}

func (r ReadValueHistory) Run(
	ctx context.Context, boards Boards, snaps Snapshots, ledger Contributions,
) (domain.ValueHistory, error) {
	run, ok, err := boards.LatestRun(ctx)
	if err != nil {
		return domain.ValueHistory{}, err
	}
	if !ok {
		return domain.ValueHistory{Points: []domain.ValuePoint{}}, r.Check(ctx, r.Cabal)
	}
	load := func(ctx context.Context) (domain.ValueHistory, error) { return r.curve(ctx, run, snaps, ledger) }
	if r.Histories == nil {
		return load(ctx)
	}
	key := HistoryKey{RunID: run.ID, Rev: run.Rev, Cabal: r.Cabal, Range: r.Range}
	return r.Histories.Load(ctx, key, load)
}

func (r ReadValueHistory) curve(
	ctx context.Context, run domain.Run, snaps Snapshots, ledger Contributions,
) (domain.ValueHistory, error) {
	if err := r.Check(ctx, r.Cabal); err != nil {
		return domain.ValueHistory{}, err
	}
	since, ok := r.Range.Start(run.AsOf)
	if !ok {
		since = time.Unix(0, 0).UTC()
	}
	rows, err := snaps.SnapshotsSince(ctx, r.Cabal, since, run.AsOf)
	if err != nil {
		return domain.ValueHistory{}, err
	}
	history, err := ledger.CabalContributionHistory(ctx, r.Cabal)
	if err != nil {
		return domain.ValueHistory{}, err
	}
	contributed := make([]domain.Contribution, 0, len(history))
	for _, p := range history {
		contributed = append(contributed, domain.Contribution{At: p.At, Net: p.NetContributed})
	}
	points, err := domain.ValueCurve(r.Range, run.AsOf, rows, contributed)
	if err != nil {
		return domain.ValueHistory{}, errs.Wrap(err, errs.CodeInternal, "ranking.ReadValueHistory")
	}
	return domain.ValueHistory{Points: points, PricesAsOf: &run.PricesAsOf}, nil
}
