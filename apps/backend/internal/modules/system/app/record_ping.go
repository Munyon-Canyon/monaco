package app

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/system/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/system/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type RecordPing struct {
	ID     uuid.UUID
	UserID ids.UserID
	Note   domain.Note
}

type RecordPingHandler struct {
	uow *db.UnitOfWork
}

func NewRecordPingHandler(uow *db.UnitOfWork) *RecordPingHandler {
	return &RecordPingHandler{uow: uow}
}

func (h *RecordPingHandler) Handle(ctx context.Context, cmd RecordPing) (Ping, error) {
	err := h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		row := sqlc.InsertPingParams{ID: cmd.ID, UserID: cmd.UserID.UUID(), Note: cmd.Note.String()}
		if err := sqlc.New(tx.Queries()).InsertPing(ctx, row); err != nil {
			return err
		}
		return tx.Events.Append(ctx, events.SystemPinged{V: 1, PingID: row.ID, UserID: row.UserID, Note: row.Note})
	})
	if err != nil {
		return Ping{}, err
	}
	return Ping{ID: cmd.ID, Note: cmd.Note.String()}, nil
}
