package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const revokeAccessOp = "cabal.RevokeAccess"

type RevokeAccess struct {
	ActorID   ids.UserID
	CabalID   ids.CabalID
	RequestID ids.AccessRequestID
}

type RevokeAccessHandler struct {
	uow   *db.UnitOfWork
	clock clock.Clock
}

func NewRevokeAccessHandler(uow *db.UnitOfWork, c clock.Clock) *RevokeAccessHandler {
	return &RevokeAccessHandler{uow: uow, clock: c}
}

func (h *RevokeAccessHandler) Handle(ctx context.Context, cmd RevokeAccess) (Access, error) {
	var revoked Access
	err := h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		cabal, actor, err := lockCabal(ctx, q, cmd.CabalID, cmd.ActorID, revokeAccessOp)
		if err != nil {
			return err
		}
		row, req, err := findRequest(ctx, q, cmd.CabalID, cmd.RequestID, revokeAccessOp)
		if err != nil {
			return err
		}
		if err := domain.CanRevoke(actor, cabal, req); err != nil {
			return err
		}
		revoked, err = settleRequest(ctx, tx, settlement{
			row: row, event: domain.AccessRevoke, actor: cmd.ActorID, at: h.clock.Now(),
		}, revokeAccessOp)
		return err
	})
	if err != nil {
		return Access{}, err
	}
	return revoked, nil
}
