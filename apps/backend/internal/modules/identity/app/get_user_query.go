package app

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
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
}

func GetUser(
	ctx context.Context, cards UserCards, follows FollowCounts, cabals CabalCounts, viewer, id ids.UserID,
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
	return PublicUser{
		ID: id, Handle: card.Handle, DisplayName: card.DisplayName, PhotoURL: card.PhotoURL,
		Bio: card.Bio, CreatedAt: card.CreatedAt, CabalCount: len(memberships),
		FollowerCount: followers, FollowingCount: following, FollowedByMe: followed, BlockedByMe: blocked,
	}, nil
}
