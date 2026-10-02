package adapters

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func (Users) InsertDevUser(
	ctx context.Context, q sqlc.DBTX, id ids.UserID, privyID, handle, displayName string, at time.Time,
) error {
	return sqlc.New(q).InsertDevUser(ctx, sqlc.InsertDevUserParams{
		ID: id.UUID(), PrivyUserID: privyID, Handle: pgtype.Text{String: handle, Valid: true},
		DisplayName: displayName, Now: at,
	})
}
