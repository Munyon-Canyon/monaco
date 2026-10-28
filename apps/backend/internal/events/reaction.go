package events

import "github.com/google/uuid"

const (
	TypeReactionAdded   Type = "reaction.added"
	TypeReactionRemoved Type = "reaction.removed"
)

const reactionAggregate = "feed_object"

type ReactionAdded struct {
	V            int        `json:"v"`
	FeedObjectID uuid.UUID  `json:"feed_object_id"`
	CommentID    *uuid.UUID `json:"comment_id"`
	UserID       uuid.UUID  `json:"user_id"        pii:"true"`
	Emoji        string     `json:"emoji"`
	ItemActorID  *uuid.UUID `json:"item_actor_id"  pii:"true"`
	CabalID      *uuid.UUID `json:"cabal_id"`
}

func (ReactionAdded) Type() Type { return TypeReactionAdded }

func (ReactionAdded) AggregateType() string { return reactionAggregate }

func (e ReactionAdded) AggregateID() uuid.UUID { return e.FeedObjectID }

type ReactionRemoved struct {
	V            int        `json:"v"`
	FeedObjectID uuid.UUID  `json:"feed_object_id"`
	CommentID    *uuid.UUID `json:"comment_id"`
	UserID       uuid.UUID  `json:"user_id"        pii:"true"`
	Emoji        string     `json:"emoji"`
	ItemActorID  *uuid.UUID `json:"item_actor_id"  pii:"true"`
	CabalID      *uuid.UUID `json:"cabal_id"`
}

func (ReactionRemoved) Type() Type { return TypeReactionRemoved }

func (ReactionRemoved) AggregateType() string { return reactionAggregate }

func (e ReactionRemoved) AggregateID() uuid.UUID { return e.FeedObjectID }
