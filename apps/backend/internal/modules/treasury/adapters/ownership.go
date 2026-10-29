package adapters

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type holdersStore interface {
	MemberSharesInCabalsOf(context.Context, uuid.UUID) ([]sqlc.MemberSharesInCabalsOfRow, error)
	ShareUnitsByCabal(context.Context, uuid.UUID) ([]sqlc.ShareUnitsByCabalRow, error)
}

func (q *Queries) Ownership(ctx context.Context, cabal ids.CabalID) (map[ids.UserID]int32, error) {
	const op = "treasury.Queries.Ownership"
	rows, err := q.holders.ShareUnitsByCabal(ctx, cabal.UUID())
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), op)
	}
	units := make([]money.SharesUnits, len(rows))
	for i, row := range rows {
		if units[i], err = shares(row.ShareUnits); err != nil {
			return nil, errs.Wrap(err, errs.CodeOf(err), op)
		}
	}
	bps, err := domain.SlicesBps(units)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), op)
	}
	out := make(map[ids.UserID]int32, len(rows))
	for i, row := range rows {
		out[ids.UserIDFrom(row.UserID)] = bps[i]
	}
	return out, nil
}

func (q *Queries) OwnershipBps(ctx context.Context, user ids.UserID) (map[ids.CabalID]int32, error) {
	const op = "treasury.Queries.OwnershipBps"
	rows, err := q.holders.MemberSharesInCabalsOf(ctx, user.UUID())
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), op)
	}
	byCabal := map[uuid.UUID][]sqlc.MemberSharesInCabalsOfRow{}
	for _, row := range rows {
		byCabal[row.CabalID] = append(byCabal[row.CabalID], row)
	}
	out := make(map[ids.CabalID]int32, len(byCabal))
	for cabal, members := range byCabal {
		bps, err := slicesOf(members)
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeOf(err), op)
		}
		for i, member := range members {
			if member.UserID == user.UUID() {
				out[ids.CabalIDFrom(cabal)] = bps[i]
			}
		}
	}
	return out, nil
}

func slicesOf(members []sqlc.MemberSharesInCabalsOfRow) ([]int32, error) {
	units := make([]money.SharesUnits, len(members))
	for i, member := range members {
		var err error
		if units[i], err = shares(member.ShareUnits); err != nil {
			return nil, err
		}
	}
	return domain.SlicesBps(units)
}
