package adapters

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

var _ port.InviteReader = Postgres{}

func (r Postgres) PendingInvites(ctx context.Context, user ids.UserID, now time.Time) ([]port.PendingInvite, error) {
	if r.users == nil {
		return nil, errs.New(errs.CodeInternal, "cabal.PendingInvites")
	}
	invites, err := app.ListMyInvites(ctx, r.db, r.users, user, now)
	if err != nil {
		return nil, err
	}
	out := make([]port.PendingInvite, len(invites))
	for i, inv := range invites {
		picture := ""
		if inv.Cabal.PictureURL != nil {
			picture = *inv.Cabal.PictureURL
		}
		out[i] = port.PendingInvite{
			ID: inv.ID, CabalID: ids.CabalIDFrom(inv.Cabal.ID), CabalName: inv.Cabal.Name, PictureURL: picture,
			MemberCount: int(inv.Cabal.MemberCount), InviterID: ids.UserIDFrom(inv.InvitedBy.UserID),
			InviterName: inv.InvitedBy.DisplayName, InviterPhotoURL: inv.InvitedBy.PhotoURL,
			CreatedAt: inv.CreatedAt, ExpiresAt: inv.ExpiresAt,
		}
	}
	slices.SortStableFunc(out, func(a, b port.PendingInvite) int { return b.CreatedAt.Compare(a.CreatedAt) })
	return out, nil
}

var _ port.InviteLookup = Postgres{}

func (r Postgres) InvitesByID(ctx context.Context, requestIDs []uuid.UUID) (map[uuid.UUID]port.ReceivedInvite, error) {
	const op = "cabal.InvitesByID"
	if r.users == nil {
		return nil, errs.New(errs.CodeInternal, op)
	}
	out := make(map[uuid.UUID]port.ReceivedInvite, len(requestIDs))
	if len(requestIDs) == 0 {
		return out, nil
	}
	rows, err := sqlc.New(r.db).InvitesByID(ctx, requestIDs)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	inviters := make([]ids.UserID, 0, len(rows))
	for _, row := range rows {
		inviters = append(inviters, ids.UserIDFrom(row.InvitedBy.Bytes))
	}
	cards, err := r.users.UsersByID(ctx, inviters)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	for _, row := range rows {
		inviter := ids.UserIDFrom(row.InvitedBy.Bytes)
		out[row.ID] = port.ReceivedInvite{
			ID: row.ID, CabalID: ids.CabalIDFrom(row.CabalID), CabalName: row.CabalName,
			PictureURL: row.CabalPictureUrl.String, MemberCount: int(row.MemberCount), InviterID: inviter,
			InviterName: cards[inviter].DisplayName, InviterPhotoURL: cards[inviter].PhotoURL,
			CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt.Time,
		}
	}
	return out, nil
}
