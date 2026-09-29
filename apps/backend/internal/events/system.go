package events

import "github.com/google/uuid"

const TypeSystemPinged Type = "system.pinged"

type SystemPinged struct {
	V      int       `json:"v"`
	PingID uuid.UUID `json:"ping_id"`
	UserID uuid.UUID `json:"user_id" pii:"true"`
	Note   string    `json:"note"    pii:"true"`
}

func (SystemPinged) Type() Type { return TypeSystemPinged }

func (SystemPinged) AggregateType() string { return "system" }

func (e SystemPinged) AggregateID() uuid.UUID { return e.PingID }
