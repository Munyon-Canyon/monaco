package app

import "context"

type UnlikeFeedItem = LikeFeedItem

type UnlikeFeedItemHandler struct{ unreact *UnreactFeedItemHandler }

func NewUnlikeFeedItemHandler(d LikeDeps) *UnlikeFeedItemHandler {
	return &UnlikeFeedItemHandler{unreact: NewUnreactFeedItemHandler(d)}
}

func (h *UnlikeFeedItemHandler) Handle(ctx context.Context, cmd UnlikeFeedItem) error {
	_, err := h.unreact.Handle(
		ctx, UnreactFeedItem{FeedObjectID: cmd.FeedObjectID, Viewer: cmd.Viewer, Emoji: likeEmoji},
	)
	return err
}
