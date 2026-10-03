package events

import (
	"time"

	"github.com/google/uuid"
)

const TypeUserDeleted Type = "user.deleted"

type UserDeleted struct {
	V      int       `json:"v"`
	UserID uuid.UUID `json:"user_id" pii:"true"`
	At     time.Time `json:"at"`
}

func (UserDeleted) Type() Type { return TypeUserDeleted }

func (UserDeleted) AggregateType() string { return userAggregate }

func (e UserDeleted) AggregateID() uuid.UUID { return e.UserID }
