package app

import (
	"cmp"
	"context"
	"math"
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

type MemberPreview struct {
	UserID      ids.UserID
	DisplayName string
	PhotoURL    string
}

const previewLimit = 3

type PortfolioView struct {
	domain.Portfolio
	Cash       money.Micros
	NetWorth   money.Micros
	Arriving   money.SignedMicros
	Cabals     map[ids.CabalID]CabalView
	Members    map[ids.CabalID]Membership
	OpenVotes  map[ids.CabalID]int
	Preview    map[ids.CabalID][]MemberPreview
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
	Users     Users
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
	if ok {
		view.markStale(values, run.AsOf)
	}
	if err = view.settle(balance.InFlightFundMicros); err != nil {
		return PortfolioView{}, errs.Wrap(err, errs.CodeInternal, "ranking.ReadPortfolio")
	}
	if len(portfolio.Rows) == 0 {
		return view, nil
	}
	return r.describe(ctx, ports, view)
}

func (v *PortfolioView) markStale(values map[ids.CabalID]domain.Snapshot, asOf time.Time) {
	for i, row := range v.Rows {
		at := values[row.CabalID].At
		v.Rows[i].Stale = at.Before(asOf)
		if v.Rows[i].Stale && !at.IsZero() {
			v.Rows[i].ValuedAt = &at
		}
	}
}

func (v *PortfolioView) settle(inFlightFund money.Micros) error {
	const op = "ranking.ReadPortfolio"
	fund, arriving := inFlightFund.Uint64(), v.Portfolio.Arriving.Int64()
	if fund > math.MaxInt64 || arriving > math.MaxInt64-int64(fund) {
		return errs.New(errs.CodeInvalidInput, op)
	}
	arriving += int64(fund)
	v.Arriving = money.SignedMicrosFromInt64(arriving)
	worth, err := v.Total.Add(v.Cash)
	if err != nil {
		return err
	}
	if arriving >= 0 {
		v.NetWorth, err = worth.Add(money.MicrosFromUint64(uint64(arriving)))
		return err
	}
	if v.NetWorth, err = worth.Sub(money.MicrosFromUint64(uint64(-arriving))); err != nil {
		v.NetWorth = money.Micros{}
	}
	return nil
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
	if view.Preview, err = previewsOf(ctx, ports.Users, held, members, r.User); err != nil {
		return PortfolioView{}, err
	}
	return view, nil
}

func previewsOf(
	ctx context.Context, users Users, held []ids.CabalID, members map[ids.CabalID][]MemberView, viewer ids.UserID,
) (map[ids.CabalID][]MemberPreview, error) {
	chosen := make(map[ids.CabalID][]ids.UserID, len(held))
	var all []ids.UserID
	for _, id := range held {
		chosen[id] = previewMembers(members[id], viewer, previewLimit)
		all = append(all, chosen[id]...)
	}
	out := make(map[ids.CabalID][]MemberPreview, len(held))
	if len(all) == 0 {
		return out, nil
	}
	cards, err := users.UsersByID(ctx, all)
	if err != nil {
		return nil, err
	}
	for id, picked := range chosen {
		for _, user := range picked {
			if card, ok := cards[user]; ok {
				out[id] = append(out[id], MemberPreview{
					UserID: user, DisplayName: card.DisplayName, PhotoURL: card.PhotoURL,
				})
			}
		}
	}
	return out, nil
}

func previewMembers(members []MemberView, viewer ids.UserID, limit int) []ids.UserID {
	others := make([]MemberView, 0, len(members))
	for _, m := range members {
		if m.UserID != viewer {
			others = append(others, m)
		}
	}
	slices.SortFunc(others, func(a, b MemberView) int {
		if c := cmp.Compare(previewRank(a), previewRank(b)); c != 0 {
			return c
		}
		if c := a.JoinedAt.Compare(b.JoinedAt); c != 0 {
			return c
		}
		return cmp.Compare(a.UserID.String(), b.UserID.String())
	})
	out := make([]ids.UserID, 0, limit)
	for _, m := range others[:min(limit, len(others))] {
		out = append(out, m.UserID)
	}
	return out
}

func previewRank(m MemberView) int {
	switch {
	case m.Role.IsAdmin():
		return 0
	case m.CanVote:
		return 1
	}
	return 2
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
