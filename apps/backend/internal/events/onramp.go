package events

import (
	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const TypeOnrampStatusChanged Type = "onramp.status_changed"

type OnrampStatusChanged struct {
	V                     int           `json:"v"`
	SessionID             uuid.UUID     `json:"session_id"`
	UserID                uuid.UUID     `json:"user_id"                 pii:"true"`
	From                  *string       `json:"from"`
	To                    string        `json:"to"`
	SuggestedAmountMicros *money.Micros `json:"suggested_amount_micros"`
	Provider              *string       `json:"provider"`
}

func (OnrampStatusChanged) Type() Type { return TypeOnrampStatusChanged }

func (OnrampStatusChanged) AggregateType() string { return "onramp_session" }

func (e OnrampStatusChanged) AggregateID() uuid.UUID { return e.SessionID }
