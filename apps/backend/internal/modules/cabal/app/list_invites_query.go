package app

import (
	"context"
	"database/sql"
	"errors"
	"maps"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	listMyInvitesOp    = "cabal.ListMyInvites"
	listCabalInvitesOp = "cabal.ListCabalInvites"
)

type CabalCard struct {
	ID          uuid.UUID
	Name        string
	PictureURL  *string
	MemberCount int32
}

type ReceivedInvite struct {
	ID        uuid.UUID
	Cabal     CabalCard
	InvitedBy Person
	ExpiresAt time.Time
}

type SentInvite struct {
	ID        uuid.UUID
	User      Person
	InvitedBy Person
	ExpiresAt time.Time
}

func ListMyInvites(
	ctx context.Context, q sqlc.DBTX, users UserCards, actor ids.UserID, now time.Time,
) ([]ReceivedInvite, error) {
	rows, err := sqlc.New(q).ListPendingInvitesForUser(ctx,
		sqlc.ListPendingInvitesForUserParams{UserID: actor.UUID(), Now: now})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, listMyInvitesOp)
	}
	inviters := map[ids.UserID]bool{}
	for _, row := range rows {
		inviters[ids.UserIDFrom(row.InvitedBy.Bytes)] = true
	}
	cards, err := users.UsersByID(ctx, slices.Collect(maps.Keys(inviters)))
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, listMyInvitesOp)
	}
	out := make([]ReceivedInvite, 0, len(rows))
	for _, row := range rows {
		out = append(out, ReceivedInvite{
			ID: row.ID,
			Cabal: CabalCard{
				ID:   row.CabalID,
				Name: row.CabalName,
				PictureURL: textPtr(
					row.CabalPictureUrl.Valid,
					row.CabalPictureUrl.String,
				),
				MemberCount: row.MemberCount,
			},
			InvitedBy: person(row.InvitedBy.Bytes, cards), ExpiresAt: row.ExpiresAt.Time,
		})
	}
	return out, nil
}

func ListCabalInvites(
	ctx context.Context, q sqlc.DBTX, users UserCards, cabalID ids.CabalID, actor ids.UserID, now time.Time,
) ([]SentInvite, error) {
	dbq := sqlc.New(q)
	if _, err := dbq.FindCabal(ctx, cabalID.UUID()); errors.Is(err, sql.ErrNoRows) {
		return nil, errs.New(errs.CodeCabalNotFound, listCabalInvitesOp)
	} else if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, listCabalInvitesOp)
	}
	_, err := dbq.FindMember(ctx, sqlc.FindMemberParams{CabalID: cabalID.UUID(), UserID: actor.UUID()})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errs.New(errs.CodeNotCabalMember, listCabalInvitesOp)
	}
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, listCabalInvitesOp)
	}
	rows, err := dbq.ListPendingInvitesForCabal(ctx,
		sqlc.ListPendingInvitesForCabalParams{CabalID: cabalID.UUID(), Now: now})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, listCabalInvitesOp)
	}
	people := map[ids.UserID]bool{}
	for _, row := range rows {
		people[ids.UserIDFrom(row.UserID)], people[ids.UserIDFrom(row.InvitedBy.Bytes)] = true, true
	}
	cards, err := users.UsersByID(ctx, slices.Collect(maps.Keys(people)))
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, listCabalInvitesOp)
	}
	out := make([]SentInvite, 0, len(rows))
	for _, row := range rows {
		out = append(out, SentInvite{
			ID: row.ID, User: person(row.UserID, cards), InvitedBy: person(row.InvitedBy.Bytes, cards),
			ExpiresAt: row.ExpiresAt.Time,
		})
	}
	return out, nil
}
