package events

import (
	"time"

	"github.com/google/uuid"
)

const TypeUserNudgeDue Type = "user.nudge_due"

type UserNudgeDue struct {
	V           int       `json:"v"`
	UserID      uuid.UUID `json:"user_id"      pii:"true"`
	Kind        string    `json:"kind"`
	NudgeNumber int       `json:"nudge_number"`
	At          time.Time `json:"at"`
}

func (UserNudgeDue) Type() Type { return TypeUserNudgeDue }

func (UserNudgeDue) AggregateType() string { return userAggregate }

func (e UserNudgeDue) AggregateID() uuid.UUID { return e.UserID }
