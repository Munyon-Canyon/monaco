package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type WriteChatMoment struct {
	CabalID   ids.CabalID
	Moment    domain.ChatMoment
	At        time.Time
	DedupeKey string
}

type WriteChatMomentHandler struct {
	d ChatDeps
}

func NewWriteChatMomentHandler(d ChatDeps) *WriteChatMomentHandler {
	return &WriteChatMomentHandler{d: d}
}

func (h *WriteChatMomentHandler) Handle(ctx context.Context, cmd WriteChatMoment) (ChatMessage, error) {
	const op = "social.WriteChatMoment"
	payload, err := cmd.Moment.JSON()
	switch {
	case err != nil:
		return ChatMessage{}, err
	case cmd.CabalID.UUID() == uuid.Nil || cmd.At.IsZero() || cmd.DedupeKey == "":
		return ChatMessage{}, errs.New(errs.CodeInvalidInput, op, slog.String("reason", "incomplete"))
	}
	id := h.d.IDs.NewV7()
	var written ChatMessage
	err = h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		row, err := sqlc.New(tx.Queries()).InsertChatMoment(ctx, sqlc.InsertChatMomentParams{
			ID: id, CabalID: cmd.CabalID.UUID(), Moment: payload, DedupeKey: cmd.DedupeKey, CreatedAt: cmd.At.UTC(),
		})
		if err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		written = ChatMessage{
			ID: row.ID, CabalID: ids.CabalIDFrom(row.CabalID), Kind: domain.ChatKindSystem,
			Moment: &cmd.Moment, CreatedAt: row.CreatedAt.UTC(),
		}
		return nil
	})
	if err != nil {
		return ChatMessage{}, err
	}
	if written.ID == id {
		h.d.Publish.moment(ctx, written)
	}
	return written, nil
}
