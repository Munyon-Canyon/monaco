package app

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func CanReact(
	ctx context.Context, reads sqlc.DBTX, members Members, viewer ids.UserID, id uuid.UUID,
) (FeedItem, error) {
	const op = "social.CanReact"
	item, err := requireVisibleItem(ctx, reads, viewer, id)
	if err != nil {
		return FeedItem{}, err
	}
	_, thread, err := ResolveThread(ctx, sqlc.New(reads), id)
	if err != nil {
		return FeedItem{}, err
	}
	allowed, _, err := CanReply(ctx, members, feed.Kind(thread.Kind), ids.CabalIDFrom(thread.CabalID.Bytes), viewer)
	if err != nil {
		return FeedItem{}, err
	}
	if !allowed {
		return FeedItem{}, errs.New(errs.CodeCommentMembersOnly, op, slog.String("feed_object_id", id.String()))
	}
	return item, nil
}
