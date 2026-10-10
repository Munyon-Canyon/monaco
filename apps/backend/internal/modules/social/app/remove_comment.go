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

type RemoveComment struct {
	CommentID uuid.UUID
	Reason    events.Reason
	Admin     ids.UserID
}

type RemoveCommentHandler struct {
	d CommentDeps
}

func NewRemoveCommentHandler(d CommentDeps) *RemoveCommentHandler {
	return &RemoveCommentHandler{d: d}
}

type commentRemoval struct {
	Deleted bool `json:"deleted"`
}

func (h *RemoveCommentHandler) Handle(ctx context.Context, cmd RemoveComment) error {
	const op = "social.RemoveComment"
	admin := cmd.Admin
	return h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		row, err := q.LockComment(ctx, cmd.CommentID)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return errs.New(errs.CodeCommentNotFound, op, slog.String("comment_id", cmd.CommentID.String()))
		case err != nil:
			return errs.Wrap(err, errs.CodeInternal, op)
		case row.Deleted:
			return nil
		}
		if _, err := q.SoftDeleteComment(ctx, sqlc.SoftDeleteCommentParams{
			At: h.d.Clock.Now().UTC(), By: admin.UUID(), Kind: CommentDeletedByAdmin, ID: cmd.CommentID,
		}); err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		if _, err := q.BumpCommentCount(ctx, sqlc.BumpCommentCountParams{Delta: -1, ID: row.FeedObjectID}); err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		err = tx.Events.Append(ctx, events.CommentDeleted{
			V: 1, CommentID: row.ID, FeedObjectID: row.FeedObjectID, DeletedBy: admin.UUID(),
			DeletedByKind: CommentDeletedByAdmin,
		})
		if err != nil {
			return err
		}
		audit, err := events.NewAdminAction(
			h.d.IDs.NewV7(), admin, events.AdminActionCommentRemove, events.AdminTargetComment, row.ID.String(),
			cmd.Reason, commentRemoval{Deleted: false}, commentRemoval{Deleted: true},
		)
		if err != nil {
			return err
		}
		tx.AfterCommit(func(ctx context.Context) { h.d.Hints.PublishHint(ctx, FeedHint, nil) })
		return tx.Events.Append(ctx, audit)
	})
}
