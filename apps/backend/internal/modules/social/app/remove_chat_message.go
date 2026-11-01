package app

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type RemoveChatMessage struct {
	MessageID uuid.UUID
	Reason    events.Reason
	Admin     ids.UserID
}

type RemoveChatMessageHandler struct {
	d ChatDeps
}

func NewRemoveChatMessageHandler(d ChatDeps) *RemoveChatMessageHandler {
	return &RemoveChatMessageHandler{d: d}
}

type chatRemoval struct {
	Deleted bool `json:"deleted"`
}

func (h *RemoveChatMessageHandler) Handle(ctx context.Context, cmd RemoveChatMessage) error {
	const op = "social.RemoveChatMessage"
	removed := false
	var cabal ids.CabalID
	var thread *ThreadUpdated
	err := h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		row, found, err := lockChatMessage(ctx, q, cmd.MessageID)
		switch {
		case err != nil:
			return err
		case !found:
			return errs.New(errs.CodeChatMessageNotFound, op, slog.String("message_id", cmd.MessageID.String()))
		case row.Deleted:
			return nil
		}
		if _, err := q.SoftDeleteChatMessage(ctx, sqlc.SoftDeleteChatMessageParams{
			At: h.d.Clock.Now().UTC(), ID: cmd.MessageID,
		}); err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		removed, cabal = true, ids.CabalIDFrom(row.CabalID)
		if row.ParentID.Valid {
			dropped, err := q.DropChatReply(ctx, row.ParentID.Bytes)
			if err != nil {
				return errs.Wrap(err, errs.CodeInternal, op)
			}
			thread = &ThreadUpdated{
				MessageID: dropped.ID, ReplyCount: dropped.ReplyCount, LastReplyAt: dropped.LastReplyAt.Time.UTC(),
			}
		}
		audit, err := events.NewAdminAction(
			h.d.IDs.NewV7(), cmd.Admin, events.AdminActionChatRemove, events.AdminTargetChat, cmd.MessageID.String(),
			cmd.Reason, chatRemoval{Deleted: false}, chatRemoval{Deleted: true},
		)
		if err != nil {
			return err
		}
		return tx.Events.Append(ctx, audit)
	})
	if err == nil && removed {
		h.d.Publish.deleted(ctx, cabal, cmd.MessageID, thread)
	}
	return err
}
