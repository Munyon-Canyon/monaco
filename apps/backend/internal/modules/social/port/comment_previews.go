package port

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type CommentAuthor struct {
	ID       ids.UserID
	Name     string
	PhotoURL string
}

type CommentPreview struct {
	FeedItemID      uuid.UUID
	CommentID       uuid.UUID
	ParentCommentID *uuid.UUID
	ReplyToID       *uuid.UUID
	RootCommentID   uuid.UUID
	ProposalID      *ids.ProposalID
	CabalID         *ids.CabalID
	Author          CommentAuthor
	Body            string
	CreatedAt       time.Time
}

type CommentPreviews interface {
	CommentPreviews(ctx context.Context, viewer ids.UserID, comments []uuid.UUID) (map[uuid.UUID]CommentPreview, error)
}
