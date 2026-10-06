package app

import (
	"cmp"
	"context"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type UserCard = identity.UserCard

type MemberCabals interface {
	CabalsOf(context.Context, ids.UserID) ([]ids.CabalID, error)
}

type SharedPorts struct {
	Users   Users
	Members MemberCabals
	Latest  LatestSnapshots
	Ledger  Contributions
	Cards   CabalCards
}

type SharedCabal struct {
	Cabal  CabalView
	Value  money.Micros
	PnL    money.SignedMicros
	Return *domain.Bps
}

type ReadSharedCabals struct {
	Viewer ids.UserID
	Other  ids.UserID
}

func (r ReadSharedCabals) Run(ctx context.Context, p SharedPorts) ([]SharedCabal, error) {
	const op = "ranking.ReadSharedCabals"
	users, err := p.Users.UsersByID(ctx, []ids.UserID{r.Other})
	if err != nil {
		return nil, err
	}
	if _, ok := users[r.Other]; !ok {
		return nil, errs.New(errs.CodeUserNotFound, op)
	}
	shared, err := r.shared(ctx, p.Members)
	if err != nil || len(shared) == 0 {
		return []SharedCabal{}, err
	}
	values, err := p.Latest.LatestValuesOf(ctx, shared)
	if err != nil {
		return nil, err
	}
	rows, err := potRows(ctx, p.Ledger, shared, values)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []SharedCabal{}, nil
	}
	return namedRows(ctx, p.Cards, rows)
}

func (r ReadSharedCabals) shared(ctx context.Context, members MemberCabals) ([]ids.CabalID, error) {
	mine, err := members.CabalsOf(ctx, r.Viewer)
	if err != nil {
		return nil, err
	}
	theirs, err := members.CabalsOf(ctx, r.Other)
	if err != nil {
		return nil, err
	}
	shared := []ids.CabalID{}
	for _, c := range mine {
		if slices.Contains(theirs, c) {
			shared = append(shared, c)
		}
	}
	return shared, nil
}

func potRows(
	ctx context.Context, ledger Contributions, shared []ids.CabalID, values map[ids.CabalID]domain.Snapshot,
) ([]SharedCabal, error) {
	rows := make([]SharedCabal, 0, len(shared))
	for _, id := range shared {
		snap, ok := values[id]
		if !ok {
			continue
		}
		history, err := ledger.CabalContributionHistory(ctx, id)
		if err != nil {
			return nil, err
		}
		contributed := make([]domain.Contribution, 0, len(history))
		for _, point := range history {
			contributed = append(contributed, domain.Contribution{At: point.At, Net: point.NetContributed})
		}
		pnl, ret, err := domain.PotLifetime(snap, contributed)
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeInternal, "ranking.ReadSharedCabals")
		}
		rows = append(rows, SharedCabal{Cabal: CabalView{ID: id}, Value: snap.Value, PnL: pnl, Return: ret})
	}
	return rows, nil
}

func namedRows(ctx context.Context, cards CabalCards, rows []SharedCabal) ([]SharedCabal, error) {
	held := make([]ids.CabalID, 0, len(rows))
	for _, row := range rows {
		held = append(held, row.Cabal.ID)
	}
	named, err := cards.Cabals(ctx, held)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		card, ok := named[rows[i].Cabal.ID]
		if !ok {
			return nil, errs.New(errs.CodeInternal, "ranking.ReadSharedCabals")
		}
		rows[i].Cabal = card
	}
	slices.SortFunc(rows, func(a, b SharedCabal) int {
		return cmp.Or(b.Value.Cmp(a.Value), cmp.Compare(a.Cabal.ID.String(), b.Cabal.ID.String()))
	})
	return rows, nil
}
