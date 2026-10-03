package app

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const listAccessRequestsOp = "cabal.ListAccessRequests"

type PendingRequest struct {
	ID        uuid.UUID
	User      Person
	CreatedAt time.Time
}

func ListAccessRequests(
	ctx context.Context, q sqlc.DBTX, users UserCards, cabalID ids.CabalID, actor ids.UserID,
) ([]PendingRequest, error) {
	dbq := sqlc.New(q)
	cabal, err := dbq.FindCabal(ctx, cabalID.UUID())
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errs.New(errs.CodeCabalNotFound, listAccessRequestsOp)
	}
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, listAccessRequestsOp)
	}
	if !(domain.Cabal{CreatorID: ids.UserIDFrom(cabal.CreatorID)}).IsCreator(actor) {
		return nil, errs.New(errs.CodeNotCabalCreator, listAccessRequestsOp)
	}
	rows, err := dbq.ListPendingRequestsForCabal(ctx, cabalID.UUID())
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, listAccessRequestsOp)
	}
	userIDs := make([]ids.UserID, 0, len(rows))
	for _, row := range rows {
		userIDs = append(userIDs, ids.UserIDFrom(row.UserID))
	}
	cards, err := users.UsersByID(ctx, userIDs)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, listAccessRequestsOp)
	}
	out := make([]PendingRequest, 0, len(rows))
	for _, row := range rows {
		out = append(out, PendingRequest{ID: row.ID, User: person(row.UserID, cards), CreatedAt: row.CreatedAt})
	}
	return out, nil
}
