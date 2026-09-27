package events

import "github.com/google/uuid"

type Type string

func (t Type) Subject() string { return "events." + string(t) }

type Event interface {
	Type() Type
	AggregateType() string
	AggregateID() uuid.UUID
}
