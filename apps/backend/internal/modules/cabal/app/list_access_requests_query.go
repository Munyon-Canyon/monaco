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
	Via       *Person
	CreatedAt time.Time
}

func ListAccessRequests(
	ctx context.Context, q sqlc.DBTX, users UserCards, cabalID ids.CabalID, actor ids.UserID,
) ([]PendingRequest, error) {
	dbq := sqlc.New(q)
	_, err := dbq.FindCabal(ctx, cabalID.UUID())
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errs.New(errs.CodeCabalNotFound, listAccessRequestsOp)
	}
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, listAccessRequestsOp)
	}
	if err := requireAdminMember(ctx, dbq, cabalID, actor); err != nil {
		return nil, err
	}
	rows, err := dbq.ListPendingRequestsForCabal(ctx, cabalID.UUID())
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, listAccessRequestsOp)
	}
	userIDs := make([]ids.UserID, 0, 2*len(rows))
	for _, row := range rows {
		userIDs = append(userIDs, ids.UserIDFrom(row.UserID))
		if row.ViaUserID.Valid {
			userIDs = append(userIDs, ids.UserIDFrom(uuid.UUID(row.ViaUserID.Bytes)))
		}
	}
	cards, err := users.UsersByID(ctx, userIDs)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, listAccessRequestsOp)
	}
	out := make([]PendingRequest, 0, len(rows))
	for _, row := range rows {
		pending := PendingRequest{ID: row.ID, User: person(row.UserID, cards), CreatedAt: row.CreatedAt}
		if row.ViaUserID.Valid {
			via := person(row.ViaUserID.Bytes, cards)
			pending.Via = &via
		}
		out = append(out, pending)
	}
	return out, nil
}

func requireAdminMember(ctx context.Context, dbq *sqlc.Queries, cabalID ids.CabalID, actor ids.UserID) error {
	member, err := dbq.FindMember(ctx, sqlc.FindMemberParams{CabalID: cabalID.UUID(), UserID: actor.UUID()})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return errs.New(errs.CodeNotCabalAdmin, listAccessRequestsOp)
	case err != nil:
		return errs.Wrap(err, errs.CodeInternal, listAccessRequestsOp)
	case !domain.Role(member.Role).IsAdmin():
		return errs.New(errs.CodeNotCabalAdmin, listAccessRequestsOp)
	}
	return nil
}
