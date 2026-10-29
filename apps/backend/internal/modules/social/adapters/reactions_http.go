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

func (h HTTP) PutCommentReaction(
	ctx context.Context, req api.PutCommentReactionRequestObject,
) (api.PutCommentReactionResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	summary, err := h.ReactComment.Handle(ctx, app.ReactComment{CommentID: req.CommentId, Viewer: me, Emoji: req.Emoji})
	if err != nil {
		return nil, err
	}
	return api.PutCommentReaction200JSONResponse(wireReactionSummary(summary)), nil
}

func (h HTTP) DeleteCommentReaction(
	ctx context.Context, req api.DeleteCommentReactionRequestObject,
) (api.DeleteCommentReactionResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	summary, err := h.UnreactComment.Handle(
		ctx,
		app.UnreactComment{CommentID: req.CommentId, Viewer: me, Emoji: req.Emoji},
	)
	if err != nil {
		return nil, err
	}
	return api.DeleteCommentReaction200JSONResponse(wireReactionSummary(summary)), nil
}
