package adapters

import (
	"context"
	"slices"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
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
