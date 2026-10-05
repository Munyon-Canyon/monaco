package events

import (
	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const TypeWithdrawalSubmitted Type = "withdrawal.submitted"

type WithdrawalSubmitted struct {
	V            int                 `json:"v"`
	WithdrawalID uuid.UUID           `json:"withdrawal_id"`
	UserID       uuid.UUID           `json:"user_id"       pii:"true"`
	AmountMicros money.Micros        `json:"amount_micros"`
	ToAddress    chain.SolanaAddress `json:"to_address"    pii:"true"`
	TxSignature  chain.Signature     `json:"tx_signature"`
}

func (WithdrawalSubmitted) Type() Type { return TypeWithdrawalSubmitted }

func (WithdrawalSubmitted) AggregateType() string { return "withdrawal" }

func (e WithdrawalSubmitted) AggregateID() uuid.UUID { return e.WithdrawalID }

const (
	TypeWithdrawalConfirmed Type = "withdrawal.confirmed"
	TypeWithdrawalFailed    Type = "withdrawal.failed"
)

type WithdrawalConfirmed struct {
	V            int                 `json:"v"`
	WithdrawalID uuid.UUID           `json:"withdrawal_id"`
	UserID       uuid.UUID           `json:"user_id"       pii:"true"`
	AmountMicros money.Micros        `json:"amount_micros"`
	ToAddress    chain.SolanaAddress `json:"to_address"    pii:"true"`
	TxSignature  chain.Signature     `json:"tx_signature"`
}

func (WithdrawalConfirmed) Type() Type { return TypeWithdrawalConfirmed }

func (WithdrawalConfirmed) AggregateType() string { return "withdrawal" }

func (e WithdrawalConfirmed) AggregateID() uuid.UUID { return e.WithdrawalID }

type WithdrawalFailed struct {
	V            int          `json:"v"`
	WithdrawalID uuid.UUID    `json:"withdrawal_id"`
	UserID       uuid.UUID    `json:"user_id"       pii:"true"`
	AmountMicros money.Micros `json:"amount_micros"`
	Code         string       `json:"code"`
}

func (WithdrawalFailed) Type() Type { return TypeWithdrawalFailed }

func (WithdrawalFailed) AggregateType() string { return "withdrawal" }

func (e WithdrawalFailed) AggregateID() uuid.UUID { return e.WithdrawalID }
