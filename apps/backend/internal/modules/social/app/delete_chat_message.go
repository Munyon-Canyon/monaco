package app

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type DeleteChatMessage struct {
	CabalID   ids.CabalID
	MessageID uuid.UUID
	Caller    ids.UserID
}

type DeleteChatMessageHandler struct {
	d ChatDeps
}

func NewDeleteChatMessageHandler(d ChatDeps) *DeleteChatMessageHandler {
	return &DeleteChatMessageHandler{d: d}
}

func (h *DeleteChatMessageHandler) Handle(ctx context.Context, cmd DeleteChatMessage) error {
	const op = "social.DeleteChatMessage"
	deleted := false
	err := h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		row, found, err := lockChatMessage(ctx, q, cmd.MessageID)
		switch {
		case err != nil:
			return err
		case !found || row.CabalID != cmd.CabalID.UUID():
			return errs.New(errs.CodeChatMessageNotFound, op, slog.String("message_id", cmd.MessageID.String()))
		case row.AuthorID != cmd.Caller.UUID():
			return errs.New(errs.CodeChatMessageNotOwned, op)
		case row.Deleted:
			return nil
		}
		if _, err := q.SoftDeleteChatMessage(ctx, sqlc.SoftDeleteChatMessageParams{
			At: h.d.Clock.Now().UTC(), ID: cmd.MessageID,
		}); err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		deleted = true
		return nil
	})
	if err == nil && deleted {
		h.d.Publish.deleted(ctx, cmd.CabalID, cmd.MessageID)
	}
	return err
}
