package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type BlockedUser struct {
	ID   ids.UserID
	Card UserCard
}

func ListBlocks(ctx context.Context, db sqlc.DBTX, users Users, viewer ids.UserID) ([]BlockedUser, error) {
	const op = "social.ListBlocks"
	rows, err := sqlc.New(db).ListUserBlocks(ctx, viewer.UUID())
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	blocked := make([]ids.UserID, len(rows))
	for i, id := range rows {
		blocked[i] = ids.UserIDFrom(id)
	}
	if len(blocked) == 0 {
		return []BlockedUser{}, nil
	}
	cards, err := users.UsersByID(ctx, blocked)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), op)
	}
	out := make([]BlockedUser, len(blocked))
	for i, id := range blocked {
		out[i] = BlockedUser{ID: id, Card: cards[id]}
	}
	return out, nil
}
