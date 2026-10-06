package exports

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/app"
)

func FollowCreated(_ context.Context, e events.FollowCreated) (app.Capture, bool, error) {
	return byUser("follow_created", e.FollowerID, map[string]any{
		"followee_id": e.FolloweeID.String(), "source": e.Source,
	})
}

func CommentCreated(_ context.Context, e events.CommentCreated) (app.Capture, bool, error) {
	return byUser("comment_created", e.AuthorID, map[string]any{
		"feed_object_id": e.FeedObjectID.String(), "item_kind": e.FeedKind, "is_reply": e.ParentCommentID != nil,
	})
}
