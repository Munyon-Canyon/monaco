package app

import (
	"context"
	"log/slog"
	"slices"
	"strings"
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

type ResumeCabal struct {
	CabalID     *ids.CabalID
	Actor       *ids.UserID
	AdminAction *events.AdminAction
}

type ResumeCabalHandler struct {
	uow   *db.UnitOfWork
	clock clock.Clock
	hints HintPublisher
}

func NewResumeCabalHandler(uow *db.UnitOfWork, c clock.Clock, hints HintPublisher) *ResumeCabalHandler {
	return &ResumeCabalHandler{uow: uow, clock: c, hints: hints}
}

func (h *ResumeCabalHandler) Handle(ctx context.Context, cmd ResumeCabal) error {
	s := pauseScope{cabal: cmd.CabalID}
	var remaining []string
	err := h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		before, err := lockScope(ctx, q, s)
		if err != nil {
			return err
		}
		resolved, err := q.ResolveOpsPauses(ctx, sqlc.ResolveOpsPausesParams{
			ResolvedAt: h.clock.Now(), ResolvedBy: optionalUser(cmd.Actor), CabalID: s.row(),
		})
		if err != nil {
			return err
		}
		remaining = slices.DeleteFunc(slices.Clone(before), func(r string) bool {
			return r == string(domain.PauseReasonOps)
		})
		if resolved == 0 {
			if cmd.AdminAction != nil {
				return errs.New(errs.CodeNoOpsPause, "funding.ResumeCabal.Handle", slog.String("scope", s.key()))
			}
			return nil
		}
		if err := settleResolved(ctx, tx, s, before, remaining, h.hints); err != nil || cmd.AdminAction == nil {
			return err
		}
		return appendAdminAction(ctx, tx, *cmd.AdminAction, before, remaining)
	})
	if err != nil {
		return err
	}
	if len(remaining) > 0 {
		return errs.New(errs.CodeCabalStillPaused, "funding.ResumeCabal.Handle",
			slog.String("scope", s.key()), slog.String("reasons", strings.Join(remaining, ",")))
	}
	return nil
}

func ResolvePause(ctx context.Context, tx db.Tx, at time.Time, hints HintPublisher, pauseID uuid.UUID) error {
	q := sqlc.New(tx.Queries())
	cabalID, err := q.PauseScope(ctx, pauseID)
	if err != nil {
		return err
	}
	s := scopeFromRow(cabalID)
	before, err := lockScope(ctx, q, s)
	if err != nil {
		return err
	}
	resolved, err := q.ResolvePause(ctx, sqlc.ResolvePauseParams{ID: pauseID, ResolvedAt: at})
	if err != nil || len(resolved) == 0 {
		return err
	}
	i := slices.Index(before, resolved[0])
	return settleResolved(ctx, tx, s, before, slices.Delete(slices.Clone(before), i, i+1), hints)
}
