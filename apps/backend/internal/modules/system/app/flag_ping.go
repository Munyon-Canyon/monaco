package app

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/system/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type FlagPing struct {
	PingID  uuid.UUID
	AdminID ids.UserID
	Reason  events.Reason
}

type FlagPingHandler struct {
	uow   *db.UnitOfWork
	ids   ids.Generator
	clock clock.Clock
}

func NewFlagPingHandler(uow *db.UnitOfWork, g ids.Generator, c clock.Clock) *FlagPingHandler {
	return &FlagPingHandler{uow: uow, ids: g, clock: c}
}

func (h *FlagPingHandler) Handle(ctx context.Context, cmd FlagPing) error {
	const op = "system.FlagPing"
	return h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		flagged, err := q.PingFlagState(ctx, cmd.PingID)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return errs.New(errs.CodeNotFound, op)
		case err != nil:
			return err
		case flagged.Valid:
			return errs.New(errs.CodeAlreadyFlagged, op)
		}
		at := h.clock.Now().UTC()
		changed, err := q.FlagPing(ctx, sqlc.FlagPingParams{ID: cmd.PingID, FlaggedAt: at})
		if err != nil {
			return err
		}
		if changed == 0 {
			return errs.New(errs.CodeAlreadyFlagged, op)
		}
		err = tx.Events.Append(ctx, events.SystemPingFlagged{
			V: 1, PingID: cmd.PingID, AdminID: cmd.AdminID.UUID(), FlaggedAt: at,
		})
		if err != nil {
			return err
		}
		audit, err := events.NewAdminAction(
			h.ids.NewV7(), cmd.AdminID, events.AdminActionPingFlag, events.AdminTargetSystemPing,
			cmd.PingID.String(), cmd.Reason, map[string]any{"flagged_at": nil}, map[string]any{"flagged_at": at})
		if err != nil {
			return err
		}
		return tx.Events.Append(ctx, audit)
	})
}
