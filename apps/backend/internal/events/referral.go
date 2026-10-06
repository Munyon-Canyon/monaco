package events

import (
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	TypeReferralAttributed Type = "referral.attributed"
	TypeReferralQualified  Type = "referral.qualified"
)

type ReferralAttributed struct {
	V            int       `json:"v"`
	ReferralID   uuid.UUID `json:"referral_id"`
	Referrer     uuid.UUID `json:"referrer_id"`
	Referee      uuid.UUID `json:"referee_id"`
	CodeKind     string    `json:"code_kind"`
	Source       string    `json:"source"`
	AttributedAt time.Time `json:"attributed_at"`
}

func (ReferralAttributed) Type() Type               { return TypeReferralAttributed }
func (ReferralAttributed) AggregateType() string    { return "referral" }
func (e ReferralAttributed) AggregateID() uuid.UUID { return e.ReferralID }

type ReferralQualified struct {
	V            int          `json:"v"`
	ReferralID   uuid.UUID    `json:"referral_id"`
	Referrer     uuid.UUID    `json:"referrer_id"`
	Referee      uuid.UUID    `json:"referee_id"`
	CabalID      uuid.UUID    `json:"cabal_id"`
	AmountMicros money.Micros `json:"amount_micros"`
	QualifiedAt  time.Time    `json:"qualified_at"`
}

func (ReferralQualified) Type() Type               { return TypeReferralQualified }
func (ReferralQualified) AggregateType() string    { return "referral" }
func (e ReferralQualified) AggregateID() uuid.UUID { return e.ReferralID }
