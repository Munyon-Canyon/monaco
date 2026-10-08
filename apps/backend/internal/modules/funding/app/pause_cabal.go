package app

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type PauseCabal struct {
	CabalID     *ids.CabalID
	Reason      domain.PauseReason
	Note        string
	Actor       *ids.UserID
	AdminAction *events.AdminAction
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
	p := newPause{
		id: h.ids.NewV7(), scope: pauseScope{cabal: cmd.CabalID}, reason: cmd.Reason, note: cmd.Note,
		actor: cmd.Actor, at: h.clock.Now(), admin: cmd.AdminAction,
	}
	err := h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return writePause(ctx, tx, p, h.hints)
	})
	return p.id, err
}

type newPause struct {
	id              uuid.UUID
	scope           pauseScope
	reason          domain.PauseReason
	note            string
	actor           *ids.UserID
	externalDeposit uuid.UUID
	at              time.Time
	admin           *events.AdminAction
}

func writePause(ctx context.Context, tx db.Tx, p newPause, hints HintPublisher) error {
	q := sqlc.New(tx.Queries())
	before, err := lockScope(ctx, q, p.scope)
	if err != nil {
		return err
	}
	if p.admin != nil && slices.Contains(before, string(domain.PauseReasonOps)) {
		return errs.New(errs.CodeAlreadyPaused, "funding.PauseCabal.Handle", slog.String("scope", p.scope.key()))
	}
	if err := q.InsertPause(ctx, sqlc.InsertPauseParams{
		ID: p.id, CabalID: p.scope.row(), Reason: string(p.reason), Note: p.note,
		CreatedBy: optionalUser(p.actor), CreatedAt: p.at,
		ExternalDepositID: p.externalDeposit,
	}); err != nil {
		return err
	}
	if len(before) == 0 {
		paused := events.CabalPaused{
			V: 1, PauseID: p.id, CabalID: p.scope.eventCabalID(), Reason: string(p.reason), Scope: p.scope.name(),
		}
		if err := tx.Events.Append(ctx, paused); err != nil {
			return err
		}
	}
	after := append(slices.Clone(before), string(p.reason))
	if p.admin != nil {
		if err := appendAdminAction(ctx, tx, *p.admin, before, after); err != nil {
			return err
		}
	}
	afterCommitPauseChanged(tx, p.scope, before, after, hints)
	return nil
}
