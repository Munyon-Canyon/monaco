package app

import (
	"cmp"
	"context"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type UserCabalPorts struct {
	Users     Users
	Blocks    Blocks
	Members   MemberCabals
	Cards     CabalCards
	Ownership Ownership
	Roles     MemberRoles
}

type UserCabal struct {
	Cabal        CabalView
	Role         Role
	CanVote      bool
	Shared       bool
	OwnershipBps *int32
}

type ReadUserCabals struct {
	Viewer ids.UserID
	Other  ids.UserID
}

func (r ReadUserCabals) Run(ctx context.Context, p UserCabalPorts) ([]UserCabal, error) {
	const op = "ranking.ReadUserCabals"
	users, err := p.Users.UsersByID(ctx, []ids.UserID{r.Other})
	if err != nil {
		return nil, err
	}
	card, ok := users[r.Other]
	if !ok {
		return nil, errs.New(errs.CodeUserNotFound, op)
	}
	if blocked, err := r.blocked(ctx, p.Blocks); err != nil || blocked {
		return []UserCabal{}, err
	}
	theirs, err := p.Members.CabalsOf(ctx, r.Other)
	if err != nil || len(theirs) == 0 {
		return []UserCabal{}, err
	}
	rows, err := r.rows(ctx, p, theirs, !card.SharePerformance && r.Other != r.Viewer)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(rows, compareUserCabals)
	return rows, nil
}

func (r ReadUserCabals) rows(
	ctx context.Context, p UserCabalPorts, theirs []ids.CabalID, hidden bool,
) ([]UserCabal, error) {
	mine, err := p.Members.CabalsOf(ctx, r.Viewer)
	if err != nil {
		return nil, err
	}
	cards, err := p.Cards.Cabals(ctx, theirs)
	if err != nil {
		return nil, err
	}
	roles, err := p.Roles.MembersOf(ctx, theirs)
	if err != nil {
		return nil, err
	}
	var owned map[ids.CabalID]int32
	if !hidden {
		if owned, err = p.Ownership.OwnershipBps(ctx, r.Other); err != nil {
			return nil, err
		}
	}
	rows := make([]UserCabal, 0, len(theirs))
	for _, id := range theirs {
		view, ok := cards[id]
		if !ok {
			return nil, errs.New(errs.CodeInternal, "ranking.ReadUserCabals")
		}
		member := membershipOf(roles[id], r.Other)
		row := UserCabal{Cabal: view, Role: member.Role, CanVote: member.CanVote, Shared: slices.Contains(mine, id)}
		if !hidden {
			bps := owned[id]
			row.OwnershipBps = &bps
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (r ReadUserCabals) blocked(ctx context.Context, blocks Blocks) (bool, error) {
	byMe, err := blocks.BlockedByMe(ctx, r.Viewer, r.Other)
	if err != nil || byMe {
		return byMe, err
	}
	return blocks.BlockedByMe(ctx, r.Other, r.Viewer)
}

func compareUserCabals(a, b UserCabal) int {
	return cmp.Or(
		boolFirst(a.Shared, b.Shared),
		cmp.Compare(bpsOrZero(b.OwnershipBps), bpsOrZero(a.OwnershipBps)),
		cmp.Compare(a.Cabal.Name, b.Cabal.Name),
		cmp.Compare(a.Cabal.ID.String(), b.Cabal.ID.String()),
	)
}

func boolFirst(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return -1
	}
	return 1
}

func bpsOrZero(p *int32) int32 {
	if p == nil {
		return 0
	}
	return *p
}
