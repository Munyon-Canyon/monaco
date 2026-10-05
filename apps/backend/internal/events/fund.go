package events

import (
	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	TypeFundSubmitted Type = "cabal.fund_submitted"
	TypeFunded        Type = "cabal.funded"
	TypeFundFailed    Type = "cabal.fund_failed"
)

type FundSubmitted struct {
	V            int             `json:"v"`
	TransferID   uuid.UUID       `json:"transfer_id"`
	CabalID      uuid.UUID       `json:"cabal_id"`
	UserID       uuid.UUID       `json:"user_id"       pii:"true"`
	AmountMicros money.Micros    `json:"amount_micros"`
	TxSignature  chain.Signature `json:"tx_signature"`
}

func (FundSubmitted) Type() Type { return TypeFundSubmitted }

func (FundSubmitted) AggregateType() string { return "fund_transfer" }

func (e FundSubmitted) AggregateID() uuid.UUID { return e.TransferID }

type Funded struct {
	V                    int               `json:"v"`
	TransferID           uuid.UUID         `json:"transfer_id"`
	CabalID              uuid.UUID         `json:"cabal_id"`
	UserID               uuid.UUID         `json:"user_id"                 pii:"true"`
	AmountMicros         money.Micros      `json:"amount_micros"`
	ShareUnits           money.SharesUnits `json:"share_units"`
	SharePriceMicros     money.Micros      `json:"share_price_micros"`
	PotValueBeforeMicros money.Micros      `json:"pot_value_before_micros"`
	TotalSharesAfter     money.SharesUnits `json:"total_shares_after"`
	TxSignature          chain.Signature   `json:"tx_signature"`
}

func (Funded) Type() Type { return TypeFunded }

func (Funded) AggregateType() string { return "fund_transfer" }

func (e Funded) AggregateID() uuid.UUID { return e.TransferID }

type FundFailed struct {
	V            int          `json:"v"`
	TransferID   uuid.UUID    `json:"transfer_id"`
	CabalID      uuid.UUID    `json:"cabal_id"`
	UserID       uuid.UUID    `json:"user_id"       pii:"true"`
	AmountMicros money.Micros `json:"amount_micros"`
	Code         string       `json:"code"`
}

func (FundFailed) Type() Type { return TypeFundFailed }

func (FundFailed) AggregateType() string { return "fund_transfer" }

func (e FundFailed) AggregateID() uuid.UUID { return e.TransferID }
