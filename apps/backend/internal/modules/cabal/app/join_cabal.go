package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const joinCabalOp = "cabal.JoinCabal"

type JoinCabal struct {
	ActorID ids.UserID
	CabalID ids.CabalID
}

type JoinCabalHandler struct {
	uow *db.UnitOfWork
}

func NewJoinCabalHandler(uow *db.UnitOfWork) *JoinCabalHandler {
	return &JoinCabalHandler{uow: uow}
}

func (h *JoinCabalHandler) Handle(ctx context.Context, cmd JoinCabal) error {
	return h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		cabal, actor, err := lockCabal(ctx, sqlc.New(tx.Queries()), cmd.CabalID, cmd.ActorID, joinCabalOp)
		if err != nil {
			return err
		}
		return domain.CanJoin(actor, cabal)
	})
}
