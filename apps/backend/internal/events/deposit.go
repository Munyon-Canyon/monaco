package events

import (
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const TypeDepositCredited Type = "deposit.credited"

type DepositCredited struct {
	V             int                 `json:"v"`
	DepositID     uuid.UUID           `json:"deposit_id"`
	UserID        uuid.UUID           `json:"user_id"        pii:"true"`
	WalletAddress chain.SolanaAddress `json:"wallet_address" pii:"true"`
	AmountMicros  money.Micros        `json:"amount_micros"`
	TxSignature   chain.Signature     `json:"tx_signature"`
	Slot          int64               `json:"slot"`
	BlockTime     *time.Time          `json:"block_time"`
}

func (DepositCredited) Type() Type { return TypeDepositCredited }

func (DepositCredited) AggregateType() string { return "deposit" }

func (e DepositCredited) AggregateID() uuid.UUID { return e.DepositID }
