package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Redriver interface {
	Redeliver(ctx context.Context, letter bus.DeadLetter, suffix string) error
}

type RedriveDeadLetter struct {
	ID      uuid.UUID
	AdminID ids.UserID
	Reason  events.Reason
}

type RedriveDeadLetterHandler struct {
	uow     *db.UnitOfWork
	reads   sqlc.DBTX
	redrive Redriver
	clock   clock.Clock
}

func NewRedriveDeadLetterHandler(
	uow *db.UnitOfWork, reads sqlc.DBTX, redrive Redriver, c clock.Clock,
) *RedriveDeadLetterHandler {
	return &RedriveDeadLetterHandler{uow: uow, reads: reads, redrive: redrive, clock: c}
}

func (h *RedriveDeadLetterHandler) Handle(ctx context.Context, cmd RedriveDeadLetter) error {
	const op = "admin.RedriveDeadLetter"
	row, err := sqlc.New(h.reads).GetDeadLetter(ctx, cmd.ID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return errs.New(errs.CodeNotFound, op)
	case err != nil:
		return errs.Wrap(err, errs.CodeDBUnavailable, op)
	case !live(row.Status):
		return errs.New(errs.CodeDeadLetterNotOpen, op)
	}
	var letter bus.DeadLetter
	if err := json.Unmarshal(row.Letter, &letter); err != nil {
		return errs.Wrap(err, errs.CodeInternal, op)
	}
	letter.Seq = uint64(max(row.StreamSeq, 0))
	suffix := fmt.Sprintf("redrive-%s-%d", cmd.ID, row.Occurrences)
	if err := h.redrive.Redeliver(ctx, letter, suffix); err != nil {
		return err
	}
	return h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		changed, err := sqlc.New(tx.Queries()).MarkRedriven(ctx, sqlc.MarkRedrivenParams{
			ID: cmd.ID, RedrivenAt: h.clock.Now(), AdminID: cmd.AdminID.UUID(), Reason: cmd.Reason.String(),
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

func live(status string) bool { return status == statusOpen || status == statusRedriven }
