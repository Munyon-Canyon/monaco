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
}

func GetUser(
	ctx context.Context, cards UserCards, follows FollowCounts, cabals CabalCounts, returns Returns,
	viewer, id ids.UserID,
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
	user.Return1MBps, user.PnL1MMicros, err = oneMonthReturn(ctx, returns, card, blocked)
	if err != nil {
		return PublicUser{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	return user, nil
}

func oneMonthReturn(ctx context.Context, returns Returns, card UserCard, blocked bool) (*int64, *int64, error) {
	ranked, err := returns.PeopleReturns(ctx, rankingport.Range1M, []ids.UserID{card.ID})
	if err != nil {
		return nil, nil, err
	}
	row, has := ranked[card.ID]
	if !has || !card.SharePerformance || blocked {
		return nil, nil, nil
	}
	pnl := row.PnlMicros.Int64()
	return row.ReturnBps, &pnl, nil
}
