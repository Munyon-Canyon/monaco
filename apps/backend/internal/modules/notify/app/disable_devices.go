package app

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/notify/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func DisableDevices(ctx context.Context, tx db.Tx, user ids.UserID, at time.Time) error {
	_, err := sqlc.New(tx.Queries()).DisableTokensForUser(ctx,
		sqlc.DisableTokensForUserParams{UserID: user.UUID(), DisabledAt: at})
	return err
}
