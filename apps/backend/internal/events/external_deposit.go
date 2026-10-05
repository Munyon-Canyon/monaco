package events

import (
	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

const TypeCabalExternalDepositDetected Type = "cabal.external_deposit_detected"

type CabalExternalDepositDetected struct {
	V                 int                 `json:"v"`
	ExternalDepositID uuid.UUID           `json:"external_deposit_id"`
	CabalID           uuid.UUID           `json:"cabal_id"`
	Signature         chain.Signature     `json:"signature"`
	Sender            chain.SolanaAddress `json:"sender"              pii:"true"`
	SenderUserID      *uuid.UUID          `json:"sender_user_id"      pii:"true"`
	Mint              chain.SolanaAddress `json:"mint"`
	AssetID           *uuid.UUID          `json:"asset_id"`
	Amount            uint64              `json:"amount,string"`
}

func (CabalExternalDepositDetected) Type() Type { return TypeCabalExternalDepositDetected }

func (CabalExternalDepositDetected) AggregateType() string { return cabalAggregate }

func (e CabalExternalDepositDetected) AggregateID() uuid.UUID { return e.CabalID }
