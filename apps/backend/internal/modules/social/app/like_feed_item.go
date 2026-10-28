package app

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type LikeFeedItem struct {
	FeedObjectID uuid.UUID
	Viewer       ids.UserID
}

type LikeDeps = ReactionDeps

const likeEmoji = "❤️"

type LikeFeedItemHandler struct{ react *ReactFeedItemHandler }

func NewLikeFeedItemHandler(d LikeDeps) *LikeFeedItemHandler {
	return &LikeFeedItemHandler{react: NewReactFeedItemHandler(d)}
}

func (h *LikeFeedItemHandler) Handle(ctx context.Context, cmd LikeFeedItem) error {
	_, err := h.react.Handle(ctx, ReactFeedItem{FeedObjectID: cmd.FeedObjectID, Viewer: cmd.Viewer, Emoji: likeEmoji})
	return err
}
