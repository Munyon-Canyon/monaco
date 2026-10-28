package app

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func requireVisibleItem(ctx context.Context, reads sqlc.DBTX, viewer ids.UserID, id uuid.UUID) (FeedItem, error) {
	view, err := GetFeedItem(ctx, reads, viewer, id, FeedFilter{})
	if err != nil {
		return FeedItem{}, err
	}
	if !view.Visible {
		return FeedItem{}, errs.New(errs.CodeFeedItemNotFound, "social.requireVisibleItem")
	}
	return view.Item, nil
}
