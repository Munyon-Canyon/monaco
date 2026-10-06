package app

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type StakePoint = treasury.StakePoint

type StakeHistory interface {
	UserStakeHistory(context.Context, ids.UserID) ([]StakePoint, error)
}

type CabalSnapshots interface {
	SnapshotsOfCabals(
		ctx context.Context, cabals []ids.CabalID, since, until time.Time,
	) (map[ids.CabalID][]domain.Snapshot, error)
}

type SnapshotReads interface {
	Snapshots
	CabalSnapshots
	LatestSnapshots
}

func stakePoints(history []StakePoint) ([]domain.StakePoint, []ids.CabalID) {
	points := make([]domain.StakePoint, 0, len(history))
	seen := map[ids.CabalID]bool{}
	cabals := []ids.CabalID{}
	for _, p := range history {
		points = append(
			points,
			domain.StakePoint{CabalID: p.CabalID, At: p.At, Shares: p.ShareUnits, Net: p.NetContributed},
		)
		if !seen[p.CabalID] {
			seen[p.CabalID] = true
			cabals = append(cabals, p.CabalID)
		}
	}
	return points, cabals
}

type ReadPnLHistory struct {
	User  ids.UserID
	Range domain.Range
}

func (r ReadPnLHistory) Run(
	ctx context.Context, boards Boards, snaps CabalSnapshots, stakes StakeHistory,
) ([]domain.PnLPoint, error) {
	run, ok, err := boards.LatestRun(ctx)
	if err != nil || !ok {
		return []domain.PnLPoint{}, err
	}
	history, err := stakes.UserStakeHistory(ctx, r.User)
	if err != nil || len(history) == 0 {
		return []domain.PnLPoint{}, err
	}
	points, cabals := stakePoints(history)
	since, ok := r.Range.Start(run.AsOf)
	if !ok {
		since = time.Unix(0, 0).UTC()
	}
	series, err := snaps.SnapshotsOfCabals(ctx, cabals, since, run.AsOf)
	if err != nil {
		return nil, err
	}
	out, err := domain.PnLCurve(r.Range, run.AsOf, points, series, logSkipped(ctx))
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "ranking.ReadPnLHistory")
	}
	return out, nil
}
