package events

import "github.com/google/uuid"

const TypeCommentCreated Type = "comment.created"

const commentAggregate = "feed_comment"

type CommentCreated struct {
	V               int        `json:"v"`
	CommentID       uuid.UUID  `json:"comment_id"`
	FeedObjectID    uuid.UUID  `json:"feed_object_id"`
	FeedKind        string     `json:"feed_kind"`
	RefType         string     `json:"ref_type"`
	RefID           uuid.UUID  `json:"ref_id"`
	CabalID         *uuid.UUID `json:"cabal_id"`
	AuthorID        uuid.UUID  `json:"author_id"         pii:"true"`
	ParentCommentID *uuid.UUID `json:"parent_comment_id"`
	ParentAuthorID  *uuid.UUID `json:"parent_author_id"  pii:"true"`
	ParentDeleted   bool       `json:"parent_deleted"`
	ReplyToUserID   *uuid.UUID `json:"reply_to_user_id"  pii:"true"`
	ItemActorID     *uuid.UUID `json:"item_actor_id"     pii:"true"`
	ProposalID      *uuid.UUID `json:"proposal_id"`
	Excerpt         string     `json:"excerpt"`
}

func (CommentCreated) Type() Type { return TypeCommentCreated }

func (CommentCreated) AggregateType() string { return commentAggregate }

func (e CommentCreated) AggregateID() uuid.UUID { return e.CommentID }
