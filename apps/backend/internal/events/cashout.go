package events

import (
	"github.com/google/uuid"

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
