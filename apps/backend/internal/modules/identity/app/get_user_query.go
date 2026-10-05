package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type FollowCounts = interface {
	Counts(context.Context, ids.UserID) (int, int, error)
	FollowedByMe(context.Context, ids.UserID, ids.UserID) (bool, error)
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

type PublicUser struct {
	ID             ids.UserID
	Handle         string
	DisplayName    string
	PhotoURL       string
	FollowerCount  int
	FollowingCount int
	FollowedByMe   bool
}

func GetUser(ctx context.Context, cards UserCards, follows FollowCounts, viewer, id ids.UserID) (PublicUser, error) {
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
	return PublicUser{
		ID: id, Handle: card.Handle, DisplayName: card.DisplayName, PhotoURL: card.PhotoURL,
		FollowerCount: followers, FollowingCount: following, FollowedByMe: followed,
	}, nil
}
