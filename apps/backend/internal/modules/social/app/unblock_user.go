package app

import (
	"context"
	"database/sql"
	"errors"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type UnblockUser struct {
	Blocker ids.UserID
	Blocked ids.UserID
}

type UnblockUserHandler struct {
	uow   *db.UnitOfWork
	clock clock.Clock
}

func NewUnblockUserHandler(uow *db.UnitOfWork, clk clock.Clock) *UnblockUserHandler {
	return &UnblockUserHandler{uow: uow, clock: clk}
}

func (h *UnblockUserHandler) Handle(ctx context.Context, cmd UnblockUser) error {
	return h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		id, err := sqlc.New(tx.Queries()).DeleteUserBlock(ctx, sqlc.DeleteUserBlockParams{
			BlockerID: cmd.Blocker.UUID(), BlockedID: cmd.Blocked.UUID(),
		})
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return errs.Wrap(err, errs.CodeInternal, "social.UnblockUser")
		}
		return tx.Events.Append(ctx, events.BlockRemoved{
			V: 1, BlockID: id, BlockerID: cmd.Blocker.UUID(), BlockedID: cmd.Blocked.UUID(),
			RemovedAt: h.clock.Now().UTC(),
		})
	})
}
