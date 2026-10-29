package app

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	rankingport "github.com/monaco/monaco/apps/backend/internal/modules/ranking/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type FollowCounts = interface {
	Counts(context.Context, ids.UserID) (int, int, error)
	FollowedByMe(context.Context, ids.UserID, ids.UserID) (bool, error)
	FollowedAmong(context.Context, ids.UserID, []ids.UserID) (map[ids.UserID]bool, error)
	BlockedByMe(context.Context, ids.UserID, ids.UserID) (bool, error)
}

type CabalCounts = interface {
	CabalsOf(ctx context.Context, user ids.UserID) ([]ids.CabalID, error)
}

type Returns = interface {
	PeopleReturns(
		ctx context.Context, rng rankingport.Range, users []ids.UserID,
	) (map[ids.UserID]rankingport.PersonReturn, error)
}

type Standings = interface {
	MonthStanding(ctx context.Context, user ids.UserID) (rankingport.Standing, error)
}

type MonthStandings struct{ Boards rankingport.CallBoards }

var _ Standings = MonthStandings{}

func (s MonthStandings) MonthStanding(ctx context.Context, user ids.UserID) (rankingport.Standing, error) {
	return s.Boards.CallStanding(ctx, user, rankingport.PeriodMonth)
}

type UserCards interface {
	UsersByID(ctx context.Context, userIDs []ids.UserID) (map[ids.UserID]UserCard, error)
}

type UnwiredFollowCounts struct{}

var _ FollowCounts = UnwiredFollowCounts{}

func (UnwiredFollowCounts) Counts(context.Context, ids.UserID) (int, int, error) {
	return 0, 0, errs.New(errs.CodeUpstreamUnavailable, "identity.UnwiredFollowCounts.Counts")
}

func (UnwiredFollowCounts) FollowedByMe(context.Context, ids.UserID, ids.UserID) (bool, error) {
	return false, errs.New(errs.CodeUpstreamUnavailable, "identity.UnwiredFollowCounts.FollowedByMe")
}

func (UnwiredFollowCounts) FollowedAmong(context.Context, ids.UserID, []ids.UserID) (map[ids.UserID]bool, error) {
	return nil, errs.New(errs.CodeUpstreamUnavailable, "identity.UnwiredFollowCounts.FollowedAmong")
}

func (UnwiredFollowCounts) BlockedByMe(context.Context, ids.UserID, ids.UserID) (bool, error) {
	return false, errs.New(errs.CodeUpstreamUnavailable, "identity.UnwiredFollowCounts.BlockedByMe")
}

type UnwiredCabalCounts struct{}

var _ CabalCounts = UnwiredCabalCounts{}

func (UnwiredCabalCounts) CabalsOf(context.Context, ids.UserID) ([]ids.CabalID, error) {
	return nil, errs.New(errs.CodeUpstreamUnavailable, "identity.UnwiredCabalCounts.CabalsOf")
}

type UnwiredReturns struct{}

var _ Returns = UnwiredReturns{}

func (UnwiredReturns) PeopleReturns(
	context.Context, rankingport.Range, []ids.UserID,
) (map[ids.UserID]rankingport.PersonReturn, error) {
	return nil, errs.New(errs.CodeUpstreamUnavailable, "identity.UnwiredReturns.PeopleReturns")
}

type UnwiredStandings struct{}

var _ Standings = UnwiredStandings{}

func (UnwiredStandings) MonthStanding(context.Context, ids.UserID) (rankingport.Standing, error) {
	return rankingport.Standing{}, errs.New(errs.CodeUpstreamUnavailable, "identity.UnwiredStandings.MonthStanding")
}

type MonthCallStanding struct {
	PeriodStart time.Time
	RateBps     *int
	Right       int
	Resolved    int
	Rank        *int
	RankedCount int
	MoreToRank  int
}

type PublicUser struct {
	ID             ids.UserID
	Handle         string
	DisplayName    string
	PhotoURL       string
	Bio            string
	CreatedAt      time.Time
	CabalCount     int
	FollowerCount  int
	FollowingCount int
	FollowedByMe   bool
	BlockedByMe    bool
	Return1MBps    *int64
	PnL1MMicros    *int64
	ReturnPrivate  bool
	Standing       *MonthCallStanding
	Crown          bool
	CabalsInCommon int
}

func GetUser(
	ctx context.Context, cards UserCards, follows FollowCounts, cabals CabalCounts, returns Returns,
	standings Standings, now time.Time, viewer, id ids.UserID,
) (PublicUser, error) {
	const op = "identity.GetUser"
	found, err := cards.UsersByID(ctx, []ids.UserID{id})
	if err != nil {
		return PublicUser{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	card, ok := found[id]
	if !ok || card.Deleted || card.AccountStatus == domain.AccountBanned {
		return PublicUser{}, errs.New(errs.CodeUserNotFound, op)
	}
	followers, following, err := follows.Counts(ctx, id)
	if err != nil {
		return PublicUser{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	followed, err := follows.FollowedByMe(ctx, viewer, id)
	if err != nil {
		return PublicUser{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	blocked, err := follows.BlockedByMe(ctx, viewer, id)
	if err != nil {
		return PublicUser{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	memberships, err := cabals.CabalsOf(ctx, id)
	if err != nil {
		return PublicUser{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	user := PublicUser{
		ID: id, Handle: card.Handle, DisplayName: card.DisplayName, PhotoURL: card.PhotoURL,
		Bio: card.Bio, CreatedAt: card.CreatedAt, CabalCount: len(memberships),
		FollowerCount: followers, FollowingCount: following, FollowedByMe: followed, BlockedByMe: blocked,
		ReturnPrivate: !card.SharePerformance,
	}
	user.Return1MBps, user.PnL1MMicros, err = oneMonthReturn(ctx, returns, card, blocked, viewer == id)
	if err != nil {
		return PublicUser{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	if user.CabalsInCommon, err = commonCabals(ctx, cabals, viewer, id, memberships); err != nil {
		return PublicUser{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	if err = user.setStanding(ctx, follows, standings, now, viewer); err != nil {
		return PublicUser{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	return user, nil
}

func (u *PublicUser) setStanding(
	ctx context.Context, follows FollowCounts, standings Standings, now time.Time, viewer ids.UserID,
) error {
	standing, shown, err := visibleStanding(ctx, follows, standings, now, viewer, u.ID, u.BlockedByMe)
	if err != nil || !shown {
		return err
	}
	u.Standing = &standing
	u.Crown = standing.Rank != nil && *standing.Rank == 1
	return nil
}

func visibleStanding(
	ctx context.Context, follows FollowCounts, standings Standings, now time.Time, viewer, id ids.UserID, blocked bool,
) (MonthCallStanding, bool, error) {
	if blocked {
		return MonthCallStanding{}, false, nil
	}
	if viewer != id {
		blockedMe, err := follows.BlockedByMe(ctx, id, viewer)
		if err != nil || blockedMe {
			return MonthCallStanding{}, false, err
		}
	}
	return monthStanding(ctx, standings, id, now)
}

func commonCabals(
	ctx context.Context, cabals CabalCounts, viewer, id ids.UserID, theirs []ids.CabalID,
) (int, error) {
	if viewer == id {
		return 0, nil
	}
	mine, err := cabals.CabalsOf(ctx, viewer)
	if err != nil {
		return 0, err
	}
	have := make(map[ids.CabalID]struct{}, len(mine))
	for _, id := range mine {
		have[id] = struct{}{}
	}
	common := 0
	for _, id := range theirs {
		if _, ok := have[id]; ok {
			common++
		}
	}
	return common, nil
}

func monthStanding(
	ctx context.Context, standings Standings, id ids.UserID, now time.Time,
) (MonthCallStanding, bool, error) {
	row, err := standings.MonthStanding(ctx, id)
	if err != nil {
		return MonthCallStanding{}, false, err
	}
	utc := now.UTC()
	monthStart := time.Date(utc.Year(), utc.Month(), 1, 0, 0, 0, 0, time.UTC)
	if row.PeriodStart.UTC().Before(monthStart) || (row.Resolved == 0 && row.Rank == nil) {
		return MonthCallStanding{}, false, nil
	}
	return MonthCallStanding{
		PeriodStart: row.PeriodStart.UTC(), RateBps: row.RateBps, Right: row.Right, Resolved: row.Resolved,
		Rank: row.Rank, RankedCount: row.Of, MoreToRank: max(0, rankingport.MinResolvedCalls-row.Resolved),
	}, true, nil
}

func oneMonthReturn(
	ctx context.Context, returns Returns, card UserCard, blocked, own bool,
) (*int64, *int64, error) {
	ranked, err := returns.PeopleReturns(ctx, rankingport.Range1M, []ids.UserID{card.ID})
	if err != nil {
		return nil, nil, err
	}
	row, has := ranked[card.ID]
	if !has || blocked || (!card.SharePerformance && !own) {
		return nil, nil, nil
	}
	pnl := row.PnlMicros.Int64()
	return row.ReturnBps, &pnl, nil
}
