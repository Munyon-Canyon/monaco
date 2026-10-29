package events

import "github.com/google/uuid"

const (
	TypeCommentCreated Type = "comment.created"
	TypeCommentDeleted Type = "comment.deleted"
)

const commentAggregate = "feed_comment"

type CommentCreated struct {
	V                int        `json:"v"`
	CommentID        uuid.UUID  `json:"comment_id"`
	FeedObjectID     uuid.UUID  `json:"feed_object_id"`
	FeedKind         string     `json:"feed_kind"`
	RefType          string     `json:"ref_type"`
	RefID            uuid.UUID  `json:"ref_id"`
	CabalID          *uuid.UUID `json:"cabal_id"`
	AuthorID         uuid.UUID  `json:"author_id"           pii:"true"`
	ParentCommentID  *uuid.UUID `json:"parent_comment_id"`
	ParentAuthorID   *uuid.UUID `json:"parent_author_id"    pii:"true"`
	ParentDeleted    bool       `json:"parent_deleted"`
	ReplyToUserID    *uuid.UUID `json:"reply_to_user_id"    pii:"true"`
	ReplyToCommentID *uuid.UUID `json:"reply_to_comment_id"`
	ReplyToAuthorID  *uuid.UUID `json:"reply_to_author_id"  pii:"true"`
	ReplyToDeleted   bool       `json:"reply_to_deleted"`
	ReplyToRootID    *uuid.UUID `json:"reply_to_root_id"`
	ReplyToExcerpt   *string    `json:"reply_to_excerpt"`
	ItemActorID      *uuid.UUID `json:"item_actor_id"       pii:"true"`
	ProposalID       *uuid.UUID `json:"proposal_id"`
	Excerpt          string     `json:"excerpt"`

	ItemKind           string    `json:"item_kind"`
	ThreadFeedObjectID uuid.UUID `json:"thread_feed_object_id"`
	ItemActorName      string    `json:"item_actor_name"       pii:"true"`
	ItemLabel          string    `json:"item_label"`
	ItemNoun           string    `json:"item_noun"`
	CabalName          *string   `json:"cabal_name"`
	ReplyRule          string    `json:"reply_rule"`
	AttachmentKind     string    `json:"attachment_kind"`
}

func (CommentCreated) Type() Type { return TypeCommentCreated }

func (CommentCreated) AggregateType() string { return commentAggregate }

func (e CommentCreated) AggregateID() uuid.UUID { return e.CommentID }

type CommentDeleted struct {
	V             int       `json:"v"`
	CommentID     uuid.UUID `json:"comment_id"`
	FeedObjectID  uuid.UUID `json:"feed_object_id"`
	DeletedBy     uuid.UUID `json:"deleted_by"      pii:"true"`
	DeletedByKind string    `json:"deleted_by_kind"`
}

func (CommentDeleted) Type() Type { return TypeCommentDeleted }

func (CommentDeleted) AggregateType() string { return commentAggregate }

func (e CommentDeleted) AggregateID() uuid.UUID { return e.CommentID }
