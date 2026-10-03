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
}

func (ChatMessagePosted) Type() Type { return TypeChatMessagePosted }

func (ChatMessagePosted) AggregateType() string { return "chat_message" }

func (e ChatMessagePosted) AggregateID() uuid.UUID { return e.MessageID }
