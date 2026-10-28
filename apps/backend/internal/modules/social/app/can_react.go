package app

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func CanReact(ctx context.Context, reads sqlc.DBTX, viewer ids.UserID, id uuid.UUID) (FeedItem, error) {
	return requireVisibleItem(ctx, reads, viewer, id)
}
