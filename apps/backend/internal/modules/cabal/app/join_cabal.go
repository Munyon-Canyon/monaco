package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const joinCabalOp = "cabal.JoinCabal"

type JoinCabal struct {
	ActorID ids.UserID
	CabalID ids.CabalID
}

type JoinCabalHandler struct {
	uow   *db.UnitOfWork
	clock clock.Clock
}

func NewJoinCabalHandler(uow *db.UnitOfWork, c clock.Clock) *JoinCabalHandler {
	return &JoinCabalHandler{uow: uow, clock: c}
}

func (h *JoinCabalHandler) Handle(ctx context.Context, cmd JoinCabal) error {
	return h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		cabal, actor, err := lockCabal(ctx, sqlc.New(tx.Queries()), cmd.CabalID, cmd.ActorID, joinCabalOp)
		if err != nil {
			return err
		}
		if err := domain.CanJoin(actor, cabal); err != nil {
			return err
		}
		return addMember(ctx, tx, newMember{
			cabalID: cmd.CabalID, userID: cmd.ActorID, rules: cabal.Rules, via: ViaOpen, at: h.clock.Now(),
		}, joinCabalOp)
	})
}
