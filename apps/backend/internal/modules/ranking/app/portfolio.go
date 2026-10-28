package app

import (
	"context"
	"slices"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type LatestSnapshots interface {
	LatestValuesOf(context.Context, []ids.CabalID) (map[ids.CabalID]domain.Snapshot, error)
}

type CabalView = cabal.View

type MemberView = cabal.MemberView

type CabalCards interface {
	Cabals(context.Context, []ids.CabalID) (map[ids.CabalID]CabalView, error)
}

type Role string

const (
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

type Membership struct {
	Role    Role
	CanVote bool
}

type PortfolioView struct {
	domain.Portfolio
	Cabals     map[ids.CabalID]CabalView
	Members    map[ids.CabalID]Membership
	ComputedAt *time.Time
	PricesAsOf *time.Time
}

type PortfolioPorts struct {
	Boards    Boards
	Latest    LatestSnapshots
	Stakes    StakeHistory
	Cards     CabalCards
	Ownership Ownership
	Roles     MemberRoles
}

type ReadPortfolio struct {
	User ids.UserID
}

func (r ReadPortfolio) Run(ctx context.Context, ports PortfolioPorts) (PortfolioView, error) {
	empty := PortfolioView{Portfolio: domain.Portfolio{Rows: []domain.PortfolioRow{}}}
	run, ok, err := ports.Boards.LatestRun(ctx)
	if err != nil || !ok {
		return empty, err
	}
	empty.ComputedAt, empty.PricesAsOf = &run.FinishedAt, &run.PricesAsOf
	history, err := ports.Stakes.UserStakeHistory(ctx, r.User)
	if err != nil || len(history) == 0 {
		return empty, err
	}
	points, cabals := stakePoints(history)
	values, err := ports.Latest.LatestValuesOf(ctx, cabals)
	if err != nil {
		return PortfolioView{}, err
	}
	portfolio, err := domain.NewPortfolio(points, values, logSkipped(ctx))
	if err != nil {
		return PortfolioView{}, errs.Wrap(err, errs.CodeInternal, "ranking.ReadPortfolio")
	}
	empty.Portfolio = portfolio
	if len(portfolio.Rows) == 0 {
		return empty, nil
	}
	return r.describe(ctx, ports, empty)
}

func (r ReadPortfolio) describe(ctx context.Context, ports PortfolioPorts, view PortfolioView) (PortfolioView, error) {
	owned, err := ports.Ownership.OwnershipBps(ctx, r.User)
	if err != nil {
		return PortfolioView{}, err
	}
	held := make([]ids.CabalID, 0, len(view.Rows))
	for i, row := range view.Rows {
		view.Rows[i].OwnershipBps = owned[row.CabalID]
		held = append(held, row.CabalID)
	}
	members, err := ports.Roles.MembersOf(ctx, held)
	if err != nil {
		return PortfolioView{}, err
	}
	view.Members = make(map[ids.CabalID]Membership, len(held))
	for _, id := range held {
		view.Members[id] = membershipOf(members[id], r.User)
	}
	if view.Cabals, err = ports.Cards.Cabals(ctx, held); err != nil {
		return PortfolioView{}, err
	}
	for _, id := range held {
		if _, ok := view.Cabals[id]; !ok {
			return PortfolioView{}, errs.New(errs.CodeInternal, "ranking.ReadPortfolio")
		}
	}
	return view, nil
}

func membershipOf(members []MemberView, user ids.UserID) Membership {
	i := slices.IndexFunc(members, func(m MemberView) bool { return m.UserID == user })
	switch {
	case i < 0:
		return Membership{Role: RoleMember}
	case members[i].Role == cabal.RoleCreator:
		return Membership{Role: RoleAdmin, CanVote: members[i].CanVote}
	}
	return Membership{Role: RoleMember, CanVote: members[i].CanVote}
}
