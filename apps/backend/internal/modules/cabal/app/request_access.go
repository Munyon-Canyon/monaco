package app

import (
	"context"

	"github.com/google/uuid"

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
	ViaCode string
}

type CodeResolver = interface {
	Resolve(ctx context.Context, code string) (ids.UserID, bool, error)
}

type UnwiredCodes struct{}

func (UnwiredCodes) Resolve(context.Context, string) (ids.UserID, bool, error) {
	return ids.UserID{}, false, nil
}

type RequestAccessHandler struct {
	uow   *db.UnitOfWork
	ids   ids.Generator
	clock clock.Clock
	codes CodeResolver
}

func NewRequestAccessHandler(
	uow *db.UnitOfWork, g ids.Generator, c clock.Clock, codes CodeResolver,
) *RequestAccessHandler {
	if codes == nil {
		codes = UnwiredCodes{}
	}
	return &RequestAccessHandler{uow: uow, ids: g, clock: c, codes: codes}
}

func (h *RequestAccessHandler) resolveVia(ctx context.Context, code string) (ids.UserID, bool) {
	if code == "" {
		return ids.UserID{}, false
	}
	via, ok, err := h.codes.Resolve(ctx, code)
	return via, ok && err == nil
}

func (h *RequestAccessHandler) Handle(ctx context.Context, cmd RequestAccess) (Access, error) {
	var filed Access
	via, hasVia := h.resolveVia(ctx, cmd.ViaCode)
	err := h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		users := []ids.UserID{cmd.ActorID}
		if hasVia {
			users = append(users, via)
		}
		cabal, actors, err := lockCabalFor(ctx, q, cmd.CabalID, requestAccessOp, users...)
		if err != nil {
			return err
		}
		actor := actors[0]
		if err := domain.CanRequest(actor, cabal); err != nil {
			return err
		}
		var viaID *uuid.UUID
		var viaParam uuid.UUID
		if hasVia && actors[1].Member {
			member := via.UUID()
			viaID = &member
			viaParam = member
		}
		id := h.ids.NewV7()
		n, err := q.InsertAccessRequest(ctx, sqlc.InsertAccessRequestParams{
			ID: id, CabalID: cmd.CabalID.UUID(), UserID: cmd.ActorID.UUID(),
			Direction: string(domain.DirectionRequest), ViaUserID: viaParam, Now: h.clock.Now(),
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
			Direction: string(domain.DirectionRequest), ActorID: cmd.ActorID.UUID(), ViaUserID: viaID,
		})
	})
	if err != nil {
		return Access{}, err
	}
	return filed, nil
}
