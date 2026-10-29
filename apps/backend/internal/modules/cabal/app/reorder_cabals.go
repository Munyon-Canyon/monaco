package app

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const reorderCabalsOp = "cabal.ReorderCabals"

type ReorderCabals struct {
	ActorID  ids.UserID
	CabalIDs []ids.CabalID
}

type ReorderCabalsHandler struct {
	uow *db.UnitOfWork
}

func NewReorderCabalsHandler(uow *db.UnitOfWork) *ReorderCabalsHandler {
	return &ReorderCabalsHandler{uow: uow}
}

func (h *ReorderCabalsHandler) Handle(ctx context.Context, cmd ReorderCabals) error {
	if err := domain.ParseCabalOrder(cmd.CabalIDs); err != nil {
		return err
	}
	return h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		locked, err := q.LockMyMemberships(ctx, cmd.ActorID.UUID())
		if err != nil {
			return errs.Wrap(err, errs.CodeInternal, reorderCabalsOp)
		}
		current := make([]domain.Membership, len(locked))
		for i, id := range locked {
			current[i] = domain.Membership{CabalID: ids.CabalIDFrom(id)}
		}
		next := domain.Reorder(current, cmd.CabalIDs)
		params := sqlc.SetMySortIndexesParams{
			UserID: cmd.ActorID.UUID(), CabalIds: make([]uuid.UUID, len(next)), SortIndexes: make([]int32, len(next)),
		}
		for i, m := range next {
			params.CabalIds[i], params.SortIndexes[i] = m.CabalID.UUID(), m.SortIndex
		}
		if err := q.SetMySortIndexes(ctx, params); err != nil {
			return errs.Wrap(err, errs.CodeInternal, reorderCabalsOp)
		}
		return nil
	})
}
