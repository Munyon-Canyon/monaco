package port

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type ExternalDeposits interface {
	UnresolvedExternalDeposits(ctx context.Context) ([]ExternalDeposit, error)
}

type ExternalDeposit struct {
	ID              uuid.UUID
	CabalID         ids.CabalID
	Sender          chain.SolanaAddress
	ReturnAddress   chain.SolanaAddress
	Mint            chain.SolanaAddress
	Amount          string
	Status          domain.ExternalDepositStatus
	BounceSignature chain.Signature
	BounceAttempts  int32
	DetectedAt      time.Time
}
