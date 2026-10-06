package app

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func rangedRanges() []domain.Range {
	return []domain.Range{domain.Range1H, domain.Range1D, domain.Range1W, domain.Range1M}
}

type rangedBook struct {
	Window
	stakes     map[MemberKey]money.SharesUnits
	shares     map[ids.CabalID]money.SharesUnits
	users      map[ids.CabalID][]ids.UserID
	snaps      map[ids.CabalID]*Snapshot
	flows      map[MemberKey][]domain.Flow
	cabalFlows map[ids.CabalID][]domain.Flow
}

func (r RunValuation) readRanged(ctx context.Context, at time.Time) ([]rangedBook, error) {
	ranges := rangedRanges()
	books := make([]rangedBook, len(ranges))
	starts := make([]time.Time, len(ranges))
	for i, rng := range ranges {
		t0, _ := rng.Start(at)
		starts[i] = t0
		books[i] = rangedBook{
			Window: Window{Range: rng, T0: t0, T1: at}, stakes: map[MemberKey]money.SharesUnits{},
			shares: map[ids.CabalID]money.SharesUnits{}, users: map[ids.CabalID][]ids.UserID{},
			snaps: map[ids.CabalID]*Snapshot{},
		}
		stakes, err := r.ports.Treasury.MemberStakesAt(ctx, t0)
		if err != nil {
			return nil, err
		}
		if err := books[i].addStakes(stakes); err != nil {
			return nil, err
		}
	}
	flows, err := r.ports.Treasury.MemberFlowsBetween(ctx, starts[len(starts)-1], at)
	if err != nil {
		return nil, err
	}
	rows, err := r.ports.Snapshots.SnapshotsAt(ctx, starts)
	if err != nil {
		return nil, err
	}
	for i := range books {
		books[i].flows, books[i].cabalFlows = RangeFlows(flows, books[i].T0, books[i].T1)
		for key := range books[i].flows {
			books[i].users[key.CabalID] = append(books[i].users[key.CabalID], key.UserID)
		}
	}
	return books, addSnapshots(books, rows)
}

func (b *rangedBook) addStakes(stakes []treasury.MemberStake) error {
	for _, stake := range stakes {
		total, err := b.shares[stake.CabalID].Add(stake.ShareUnits)
		if err != nil {
			return err
		}
		b.shares[stake.CabalID] = total
		b.stakes[MemberKey{UserID: stake.UserID, CabalID: stake.CabalID}] = stake.ShareUnits
		b.users[stake.CabalID] = append(b.users[stake.CabalID], stake.UserID)
	}
	return nil
}

func addSnapshots(books []rangedBook, rows []sqlc.SnapshotsAtRow) error {
	for _, row := range rows {
		if row.Bucket < 0 || int(row.Bucket) >= len(books) || row.ValueMicros < 0 || row.TotalShares < 0 {
			return errs.New(errs.CodeDecodeFailed, "ranking.RunValuation.readRanged")
		}
		books[row.Bucket].snaps[ids.CabalIDFrom(row.CabalID)] = &Snapshot{
			At:          row.At,
			Value:       money.MicrosFromUint64(uint64(row.ValueMicros)),
			TotalShares: money.SharesUnitsFromUint64(uint64(row.TotalShares)),
		}
	}
	return nil
}
