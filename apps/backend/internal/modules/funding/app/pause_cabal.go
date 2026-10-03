package app

import (
	"context"
	"slices"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type PauseCabal struct {
	CabalID *ids.CabalID
	Reason  domain.PauseReason
	Note    string
	Actor   *ids.UserID
}

type PauseCabalHandler struct {
	uow   *db.UnitOfWork
	ids   ids.Generator
	clock clock.Clock
	hints HintPublisher
}

func NewPauseCabalHandler(uow *db.UnitOfWork, g ids.Generator, c clock.Clock, hints HintPublisher) *PauseCabalHandler {
	return &PauseCabalHandler{uow: uow, ids: g, clock: c, hints: hints}
}

func (h *PauseCabalHandler) Handle(ctx context.Context, cmd PauseCabal) (uuid.UUID, error) {
	id, s := h.ids.NewV7(), pauseScope{cabal: cmd.CabalID}
	err := h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		before, err := lockScope(ctx, q, s)
		if err != nil {
			return err
		}
		if err := q.InsertPause(ctx, sqlc.InsertPauseParams{
			ID: id, CabalID: s.row(), Reason: string(cmd.Reason), Note: cmd.Note,
			CreatedBy: optionalUser(cmd.Actor), CreatedAt: h.clock.Now(),
		}); err != nil {
			return err
		}
		if len(before) == 0 {
			paused := events.CabalPaused{
				V: 1, PauseID: id, CabalID: s.eventCabalID(), Reason: string(cmd.Reason), Scope: s.name(),
			}
			if err := tx.Events.Append(ctx, paused); err != nil {
				return err
			}
		}
		afterCommitPauseChanged(tx, s, before, append(slices.Clone(before), string(cmd.Reason)), h.hints)
		return nil
	})
	return id, err
}
