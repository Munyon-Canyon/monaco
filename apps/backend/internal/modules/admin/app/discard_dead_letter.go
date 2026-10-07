package app

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type DiscardDeadLetter struct {
	ID      uuid.UUID
	AdminID ids.UserID
	Reason  events.Reason
}

type DiscardDeadLetterHandler struct {
	uow   *db.UnitOfWork
	clock clock.Clock
}

func NewDiscardDeadLetterHandler(uow *db.UnitOfWork, c clock.Clock) *DiscardDeadLetterHandler {
	return &DiscardDeadLetterHandler{uow: uow, clock: c}
}

func (h *DiscardDeadLetterHandler) Handle(ctx context.Context, cmd DiscardDeadLetter) error {
	const op = "admin.DiscardDeadLetter"
	return h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		if _, err := q.GetDeadLetter(ctx, cmd.ID); errors.Is(err, sql.ErrNoRows) {
			return errs.New(errs.CodeNotFound, op)
		} else if err != nil {
			return err
		}
		changed, err := q.MarkDiscarded(ctx, sqlc.MarkDiscardedParams{
			ID: cmd.ID, ResolvedAt: h.clock.Now(), AdminID: cmd.AdminID.UUID(), Reason: cmd.Reason.String(),
		})
		if err != nil {
			return err
		}
		if changed == 0 {
			return errs.New(errs.CodeDeadLetterNotOpen, op)
		}
		return nil
	})
}
