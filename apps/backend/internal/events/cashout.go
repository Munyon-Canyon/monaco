package events

import (
	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const TypeCashOutStarted Type = "cashout.started"

type CashOutStarted struct {
	V            int          `json:"v"`
	JobID        uuid.UUID    `json:"job_id"`
	CabalID      uuid.UUID    `json:"cabal_id"`
	UserID       uuid.UUID    `json:"user_id"            pii:"true"`
	ShareUnits   uint64       `json:"share_units,string"`
	PayoutMicros money.Micros `json:"payout_micros"`
	SellUSDC     money.Micros `json:"sell_usdc_micros"`
}

func (CashOutStarted) Type() Type { return TypeCashOutStarted }

func (CashOutStarted) AggregateType() string { return "cash_out" }

func (e CashOutStarted) AggregateID() uuid.UUID { return e.JobID }

const (
	TypeCashOutCompleted Type = "cashout.completed"
	TypeCashOutFailed    Type = "cashout.failed"
	TypeCashOutPartial   Type = "cashout.partial"
)

type CashOutCompleted struct {
	V            int             `json:"v"`
	JobID        uuid.UUID       `json:"job_id"`
	CabalID      uuid.UUID       `json:"cabal_id"`
	UserID       uuid.UUID       `json:"user_id"            pii:"true"`
	ShareUnits   uint64          `json:"share_units,string"`
	PayoutMicros money.Micros    `json:"payout_micros"`
	Signature    chain.Signature `json:"signature"`
}

func (CashOutCompleted) Type() Type { return TypeCashOutCompleted }

func (CashOutCompleted) AggregateType() string { return "cash_out" }

func (e CashOutCompleted) AggregateID() uuid.UUID { return e.JobID }

type CashOutFailed struct {
	V          int       `json:"v"`
	JobID      uuid.UUID `json:"job_id"`
	CabalID    uuid.UUID `json:"cabal_id"`
	UserID     uuid.UUID `json:"user_id"            pii:"true"`
	ShareUnits uint64    `json:"share_units,string"`
	Code       string    `json:"code"`
}

func (CashOutFailed) Type() Type { return TypeCashOutFailed }

func (CashOutFailed) AggregateType() string { return "cash_out" }

func (e CashOutFailed) AggregateID() uuid.UUID { return e.JobID }

type CashOutPartial struct {
	V                  int             `json:"v"`
	JobID              uuid.UUID       `json:"job_id"`
	CabalID            uuid.UUID       `json:"cabal_id"`
	UserID             uuid.UUID       `json:"user_id"                     pii:"true"`
	ShareUnitsBurned   uint64          `json:"share_units_burned,string"`
	ShareUnitsReturned uint64          `json:"share_units_returned,string"`
	PayoutMicros       money.Micros    `json:"payout_micros"`
	Signature          chain.Signature `json:"signature"`
}

func (CashOutPartial) Type() Type { return TypeCashOutPartial }

func (CashOutPartial) AggregateType() string { return "cash_out" }

func (e CashOutPartial) AggregateID() uuid.UUID { return e.JobID }
