package adapters

import (
	"context"
	"database/sql"
	"errors"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func (r Postgres) InvitePreview(
	ctx context.Context, cabal ids.CabalID, inviter ids.UserID,
) (port.CabalPreview, bool, error) {
	const op = "cabal.InvitePreview"
	row, err := r.q.InvitePreview(ctx, sqlc.InvitePreviewParams{CabalID: cabal.UUID(), Inviter: inviter.UUID()})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return port.CabalPreview{}, false, nil
	case err != nil:
		return port.CabalPreview{}, false, errs.Wrap(err, errs.CodeInternal, op)
	}
	first := make([]ids.UserID, 0, len(row.FirstMemberIds))
	for _, raw := range row.FirstMemberIds {
		id, err := userID(op, raw)
		if err != nil {
			return port.CabalPreview{}, false, err
		}
		first = append(first, id)
	}
	return port.CabalPreview{
		ID: cabal, Name: row.Name, PictureURL: row.PictureUrl.String, MemberCount: row.MemberCount,
		CreatedAt: row.CreatedAt.UTC(), JoinMode: domain.JoinMode(row.JoinMode),
		Banned: port.Status(row.Status) == port.StatusBanned, InviterIsMember: row.InviterIsMember,
		FirstMemberIDs: first,
	}, true, nil
}
