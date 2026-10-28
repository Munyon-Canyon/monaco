package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
)

func (h HTTP) PutFeedReaction(
	ctx context.Context, req api.PutFeedReactionRequestObject,
) (api.PutFeedReactionResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	summary, err := h.ReactFeedItem.Handle(ctx, app.ReactFeedItem{
		FeedObjectID: req.Id, Viewer: me, Emoji: req.Emoji,
	})
	if err != nil {
		return nil, err
	}
	return api.PutFeedReaction200JSONResponse(wireReactionSummary(summary)), nil
}

func (h HTTP) DeleteFeedReaction(
	ctx context.Context, req api.DeleteFeedReactionRequestObject,
) (api.DeleteFeedReactionResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	summary, err := h.UnreactFeedItem.Handle(ctx, app.UnreactFeedItem{
		FeedObjectID: req.Id, Viewer: me, Emoji: req.Emoji,
	})
	if err != nil {
		return nil, err
	}
	return api.DeleteFeedReaction200JSONResponse(wireReactionSummary(summary)), nil
}

func wireReactionSummary(summary app.ReactionSummary) api.ReactionSummary {
	return api.ReactionSummary{Reactions: wireReactions(summary.Reactions), Total: summary.Total}
}
