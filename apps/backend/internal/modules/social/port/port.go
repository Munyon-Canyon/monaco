package port

import (
	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type TopReply struct {
	CommentID     uuid.UUID
	ProposalID    uuid.UUID
	AuthorID      ids.UserID
	Excerpt       string
	ReactionCount int
	TopEmoji      string
	TopEmojiCount int
}
