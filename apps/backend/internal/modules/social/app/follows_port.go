package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type FollowsPort interface {
	Counts(ctx context.Context, user ids.UserID) (followers, following int, err error)
	FollowedByMe(ctx context.Context, viewer, user ids.UserID) (bool, error)
	FollowingIDs(ctx context.Context, user ids.UserID) ([]ids.UserID, error)
}

type Follows struct{ db sqlc.DBTX }

func NewFollows(db sqlc.DBTX) Follows { return Follows{db: db} }

var _ FollowsPort = Follows{}

func (f Follows) Counts(ctx context.Context, user ids.UserID) (int, int, error) {
	const op = "social.FollowsPort.Counts"
	row, err := sqlc.New(f.db).CountFollows(ctx, user.UUID())
	if err != nil {
		return 0, 0, errs.Wrap(err, errs.CodeOf(err), op)
	}
	return int(row.Followers), int(row.Following), nil
}

func (f Follows) FollowedByMe(ctx context.Context, viewer, user ids.UserID) (bool, error) {
	const op = "social.FollowsPort.FollowedByMe"
	followed, err := sqlc.New(f.db).IsFollowing(ctx, sqlc.IsFollowingParams{
		FollowerID: viewer.UUID(), FolloweeID: user.UUID(),
	})
	if err != nil {
		return false, errs.Wrap(err, errs.CodeOf(err), op)
	}
	return followed, nil
}

func (f Follows) FollowingIDs(ctx context.Context, user ids.UserID) ([]ids.UserID, error) {
	const op = "social.FollowsPort.FollowingIDs"
	rows, err := sqlc.New(f.db).FollowingIDs(ctx, user.UUID())
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), op)
	}
	out := make([]ids.UserID, len(rows))
	for i, id := range rows {
		out[i] = ids.UserIDFrom(id)
	}
	return out, nil
}
