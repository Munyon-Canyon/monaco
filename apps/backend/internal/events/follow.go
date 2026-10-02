package events

import (
	"time"

	"github.com/google/uuid"
)

const (
	TypeFollowCreated Type = "follow.created"
	TypeFollowRemoved Type = "follow.removed"
)

const followAggregate = "follow"

type FollowCreated struct {
	V          int       `json:"v"`
	FollowID   uuid.UUID `json:"follow_id"`
	FollowerID uuid.UUID `json:"follower_id" pii:"true"`
	FolloweeID uuid.UUID `json:"followee_id" pii:"true"`
	Source     string    `json:"source"`
	CreatedAt  time.Time `json:"created_at"`
}

func (FollowCreated) Type() Type { return TypeFollowCreated }

func (FollowCreated) AggregateType() string { return followAggregate }

func (e FollowCreated) AggregateID() uuid.UUID { return e.FollowID }

type FollowRemoved struct {
	V          int       `json:"v"`
	FollowID   uuid.UUID `json:"follow_id"`
	FollowerID uuid.UUID `json:"follower_id" pii:"true"`
	FolloweeID uuid.UUID `json:"followee_id" pii:"true"`
	RemovedAt  time.Time `json:"removed_at"`
}

func (FollowRemoved) Type() Type { return TypeFollowRemoved }

func (FollowRemoved) AggregateType() string { return followAggregate }

func (e FollowRemoved) AggregateID() uuid.UUID { return e.FollowID }
