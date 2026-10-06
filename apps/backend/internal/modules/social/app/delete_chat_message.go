package app

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type DeleteChatMessage struct {
	CabalID   ids.CabalID
	MessageID uuid.UUID
	Caller    ids.UserID
}

type DeleteChatMessageHandler struct {
	uow   *db.UnitOfWork
	clock clock.Clock
}

func NewDeleteChatMessageHandler(uow *db.UnitOfWork, clk clock.Clock) *DeleteChatMessageHandler {
	return &DeleteChatMessageHandler{uow: uow, clock: clk}
}

func (h *DeleteChatMessageHandler) Handle(ctx context.Context, cmd DeleteChatMessage) error {
	const op = "social.DeleteChatMessage"
	return h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
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
			At: h.clock.Now().UTC(), ID: cmd.MessageID,
		}); err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		return nil
	})
}
