package adapters

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

type DeletedCleanup struct{}

func (DeletedCleanup) Handle(ctx context.Context, tx db.Tx, e events.UserDeleted, _ time.Time) error {
	const op = "social.DeletedCleanup"
	q := sqlc.New(tx.Queries())
	for _, del := range []func(context.Context, uuid.UUID) error{
		q.DeleteUserItemReactions, q.DeleteUserItemLikes, q.DeleteUserCommentReactions, q.DeleteUserPollVotes,
	} {
		if err := del(ctx, e.UserID); err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
	}
	return nil
}
