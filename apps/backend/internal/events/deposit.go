package events

import (
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	TypeDepositCredited           Type = "deposit.credited"
	TypeDepositCandidateSeen      Type = "deposit.candidate_seen"
	TypeDepositCandidateDismissed Type = "deposit.candidate_dismissed"
)

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

type DepositCandidateSeen struct {
	V             int                 `json:"v"`
	CandidateID   uuid.UUID           `json:"candidate_id"`
	UserID        uuid.UUID           `json:"user_id"        pii:"true"`
	WalletAddress chain.SolanaAddress `json:"wallet_address" pii:"true"`
	TxSignature   chain.Signature     `json:"tx_signature"`
	Slot          int64               `json:"slot"`
	BlockTime     *time.Time          `json:"block_time"`
	Source        string              `json:"source"`
}

func (DepositCandidateSeen) Type() Type { return TypeDepositCandidateSeen }

func (DepositCandidateSeen) AggregateType() string { return "deposit" }

func (e DepositCandidateSeen) AggregateID() uuid.UUID { return e.CandidateID }

type DepositCandidateDismissed struct {
	V             int                 `json:"v"`
	CandidateID   uuid.UUID           `json:"candidate_id"`
	UserID        uuid.UUID           `json:"user_id"        pii:"true"`
	WalletAddress chain.SolanaAddress `json:"wallet_address" pii:"true"`
	TxSignature   chain.Signature     `json:"tx_signature"`
	Reason        string              `json:"reason"`
}

func (DepositCandidateDismissed) Type() Type { return TypeDepositCandidateDismissed }

func (DepositCandidateDismissed) AggregateType() string { return "deposit" }

func (e DepositCandidateDismissed) AggregateID() uuid.UUID { return e.CandidateID }
