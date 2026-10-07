package events

import (
	"time"

	"github.com/google/uuid"
)

const (
	TypeBlockCreated Type = "block.created"
	TypeBlockRemoved Type = "block.removed"
)

const blockAggregate = "user_block"

type BlockCreated struct {
	V         int       `json:"v"`
	BlockID   uuid.UUID `json:"block_id"`
	BlockerID uuid.UUID `json:"blocker_id" pii:"true"`
	BlockedID uuid.UUID `json:"blocked_id" pii:"true"`
	CreatedAt time.Time `json:"created_at"`
}

func (BlockCreated) Type() Type { return TypeBlockCreated }

func (BlockCreated) AggregateType() string { return blockAggregate }

func (e BlockCreated) AggregateID() uuid.UUID { return e.BlockID }

type BlockRemoved struct {
	V         int       `json:"v"`
	BlockID   uuid.UUID `json:"block_id"`
	BlockerID uuid.UUID `json:"blocker_id" pii:"true"`
	BlockedID uuid.UUID `json:"blocked_id" pii:"true"`
	RemovedAt time.Time `json:"removed_at"`
}

func (BlockRemoved) Type() Type { return TypeBlockRemoved }

func (BlockRemoved) AggregateType() string { return blockAggregate }

func (e BlockRemoved) AggregateID() uuid.UUID { return e.BlockID }
