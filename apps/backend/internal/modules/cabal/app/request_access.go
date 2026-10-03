package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const requestAccessOp = "cabal.RequestAccess"

type RequestAccess struct {
	ActorID ids.UserID
	CabalID ids.CabalID
}

type RequestAccessHandler struct {
	uow   *db.UnitOfWork
	ids   ids.Generator
	clock clock.Clock
}

func NewRequestAccessHandler(uow *db.UnitOfWork, g ids.Generator, c clock.Clock) *RequestAccessHandler {
	return &RequestAccessHandler{uow: uow, ids: g, clock: c}
}

func (h *RequestAccessHandler) Handle(ctx context.Context, cmd RequestAccess) (Access, error) {
	var filed Access
	err := h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		cabal, actor, err := lockCabal(ctx, q, cmd.CabalID, cmd.ActorID, requestAccessOp)
		if err != nil {
			return err
		}
		if err := domain.CanRequest(actor, cabal); err != nil {
			return err
		}
		id := h.ids.NewV7()
		n, err := q.InsertAccessRequest(ctx, sqlc.InsertAccessRequestParams{
			ID: id, CabalID: cmd.CabalID.UUID(), UserID: cmd.ActorID.UUID(),
			Direction: string(domain.DirectionRequest), Now: h.clock.Now(),
		})
		if err != nil {
			return errs.Wrap(err, errs.CodeInternal, requestAccessOp)
		}
		if n == 0 {
			return errs.New(errs.CodeRequestPending, requestAccessOp)
		}
		filed = Access{ID: id, Direction: string(domain.DirectionRequest), Status: string(domain.AccessPending)}
		return tx.Events.Append(ctx, events.CabalAccessRequested{
			V: 1, RequestID: id, CabalID: cmd.CabalID.UUID(), UserID: cmd.ActorID.UUID(),
			Direction: string(domain.DirectionRequest), ActorID: cmd.ActorID.UUID(),
		})
	})
	if err != nil {
		return Access{}, err
	}
	return filed, nil
}
