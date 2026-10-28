package app

import (
	"context"
	"slices"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
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

type OpenVoteCounts interface {
	OpenVoteCounts(context.Context, []ids.CabalID) (map[ids.CabalID]int, error)
}

type PortfolioView struct {
	domain.Portfolio
	Cash       money.Micros
	NetWorth   money.Micros
	Cabals     map[ids.CabalID]CabalView
	Members    map[ids.CabalID]Membership
	OpenVotes  map[ids.CabalID]int
	ComputedAt *time.Time
	PricesAsOf *time.Time
}

type PortfolioPorts struct {
	Balances  DisplayBalances
	Joined    MemberCabals
	Votes     OpenVoteCounts
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
	view := PortfolioView{Portfolio: domain.Portfolio{Rows: []domain.PortfolioRow{}}}
	balance, err := ports.Balances.ForDisplay(ctx, r.User)
	if err != nil {
		return PortfolioView{}, err
	}
	cash := balance.AvailableMicros
	memberships, err := ports.Joined.CabalsOf(ctx, r.User)
	if err != nil {
		return PortfolioView{}, err
	}
	view.Cash = cash
	run, ok, err := ports.Boards.LatestRun(ctx)
	if err != nil {
		return PortfolioView{}, err
	}
	var points []domain.StakePoint
	var cabals []ids.CabalID
	if ok {
		view.ComputedAt, view.PricesAsOf = &run.FinishedAt, &run.PricesAsOf
		history, err := ports.Stakes.UserStakeHistory(ctx, r.User)
		if err != nil {
			return PortfolioView{}, err
		}
		points, cabals = stakePoints(history)
		cabals = appendNew(cabals, memberships)
	}
	values := map[ids.CabalID]domain.Snapshot{}
	if len(cabals) > 0 {
		if values, err = ports.Latest.LatestValuesOf(ctx, cabals); err != nil {
			return PortfolioView{}, err
		}
	}
	portfolio, err := domain.NewPortfolio(points, values, memberships, logSkipped(ctx))
	if err != nil {
		return PortfolioView{}, errs.Wrap(err, errs.CodeInternal, "ranking.ReadPortfolio")
	}
	view.Portfolio = portfolio
	if view.NetWorth, err = portfolio.Total.Add(cash); err != nil {
		return PortfolioView{}, errs.Wrap(err, errs.CodeInternal, "ranking.ReadPortfolio")
	}
	if len(portfolio.Rows) == 0 {
		return view, nil
	}
	return r.describe(ctx, ports, view)
}

func appendNew(cabals, more []ids.CabalID) []ids.CabalID {
	for _, id := range more {
		if !slices.Contains(cabals, id) {
			cabals = append(cabals, id)
		}
	}
	return cabals
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
	if view.OpenVotes, err = ports.Votes.OpenVoteCounts(ctx, held); err != nil {
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
	case members[i].Role.IsAdmin():
		return Membership{Role: RoleAdmin, CanVote: members[i].CanVote}
	}
	return Membership{Role: RoleMember, CanVote: members[i].CanVote}
}
