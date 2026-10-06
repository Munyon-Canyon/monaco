package events

import (
	"time"

	"github.com/google/uuid"
)

const (
	TypeSystemPinged      Type = "system.pinged"
	TypeSystemPingFlagged Type = "system.ping_flagged"
)

type SystemPinged struct {
	V      int       `json:"v"`
	PingID uuid.UUID `json:"ping_id"`
	UserID uuid.UUID `json:"user_id" pii:"true"`
	Note   string    `json:"note"    pii:"true"`
}

func (SystemPinged) Type() Type { return TypeSystemPinged }

func (SystemPinged) AggregateType() string { return "system" }

func (e SystemPinged) AggregateID() uuid.UUID { return e.PingID }

type SystemPingFlagged struct {
	V         int       `json:"v"`
	PingID    uuid.UUID `json:"ping_id"`
	AdminID   uuid.UUID `json:"admin_id"   pii:"true"`
	FlaggedAt time.Time `json:"flagged_at"`
}

func (SystemPingFlagged) Type() Type { return TypeSystemPingFlagged }

func (SystemPingFlagged) AggregateType() string { return "system" }

func (e SystemPingFlagged) AggregateID() uuid.UUID { return e.PingID }
