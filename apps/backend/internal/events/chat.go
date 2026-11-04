package events

import (
	"time"

	"github.com/google/uuid"
)

const TypeChatMessagePosted Type = "chat.message_posted"

type ChatMessagePosted struct {
	V             int        `json:"v"`
	MessageID     uuid.UUID  `json:"message_id"`
	CabalID       uuid.UUID  `json:"cabal_id"`
	AuthorID      uuid.UUID  `json:"author_id"       pii:"true"`
	ParentID      *uuid.UUID `json:"parent_id"`
	AlsoInChannel bool       `json:"also_in_channel"`
	CreatedAt     time.Time  `json:"created_at"`

	MentionedUserIDs     []uuid.UUID `json:"mentioned_user_ids"     pii:"true"`
	ThreadParticipantIDs []uuid.UUID `json:"thread_participant_ids" pii:"true"`
}

func (ChatMessagePosted) Type() Type { return TypeChatMessagePosted }

func (ChatMessagePosted) AggregateType() string { return "chat_message" }

func (e ChatMessagePosted) AggregateID() uuid.UUID { return e.MessageID }

const (
	TypeChatReactionAdded   Type = "chat.reaction_added"
	TypeChatReactionRemoved Type = "chat.reaction_removed"
)

type ChatReactionAdded struct {
	V         int        `json:"v"`
	MessageID uuid.UUID  `json:"message_id"`
	CabalID   uuid.UUID  `json:"cabal_id"`
	UserID    uuid.UUID  `json:"user_id"    pii:"true"`
	Emoji     string     `json:"emoji"`
	AuthorID  *uuid.UUID `json:"author_id"  pii:"true"`
}

func (ChatReactionAdded) Type() Type { return TypeChatReactionAdded }

func (ChatReactionAdded) AggregateType() string { return "chat_message" }

func (e ChatReactionAdded) AggregateID() uuid.UUID { return e.MessageID }

type ChatReactionRemoved struct {
	V         int        `json:"v"`
	MessageID uuid.UUID  `json:"message_id"`
	CabalID   uuid.UUID  `json:"cabal_id"`
	UserID    uuid.UUID  `json:"user_id"    pii:"true"`
	Emoji     string     `json:"emoji"`
	AuthorID  *uuid.UUID `json:"author_id"  pii:"true"`
}

func (ChatReactionRemoved) Type() Type { return TypeChatReactionRemoved }

func (ChatReactionRemoved) AggregateType() string { return "chat_message" }

func (e ChatReactionRemoved) AggregateID() uuid.UUID { return e.MessageID }
