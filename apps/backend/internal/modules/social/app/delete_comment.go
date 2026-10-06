package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type DeleteComment struct {
	CommentID uuid.UUID
	Caller    ids.UserID
}

type DeleteCommentHandler struct {
	d CommentDeps
}

func NewDeleteCommentHandler(d CommentDeps) *DeleteCommentHandler {
	return &DeleteCommentHandler{d: d}
}

func (h *DeleteCommentHandler) Handle(ctx context.Context, cmd DeleteComment) error {
	const op = "social.DeleteComment"
	return h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		row, err := q.LockComment(ctx, cmd.CommentID)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return errs.New(errs.CodeCommentNotFound, op, slog.String("comment_id", cmd.CommentID.String()))
		case err != nil:
			return errs.Wrap(err, errs.CodeInternal, op)
		case row.AuthorID != cmd.Caller.UUID():
			return errs.New(errs.CodeCommentNotAuthor, op)
		case row.Deleted:
			return nil
		}
		if _, err := q.SoftDeleteComment(ctx, sqlc.SoftDeleteCommentParams{
			At: h.d.Clock.Now().UTC(), By: cmd.Caller.UUID(), ID: cmd.CommentID,
		}); err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		if _, err := q.BumpCommentCount(ctx, sqlc.BumpCommentCountParams{Delta: -1, ID: row.FeedObjectID}); err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		tx.AfterCommit(func(ctx context.Context) { h.d.Hints.PublishHint(ctx, FeedHint, nil) })
		return tx.Events.Append(ctx, events.CommentDeleted{
			V: 1, CommentID: row.ID, FeedObjectID: row.FeedObjectID, DeletedBy: cmd.Caller.UUID(),
		})
	})
}
