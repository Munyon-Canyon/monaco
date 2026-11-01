package port

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type ArrivalBand struct {
	ID      uuid.UUID
	Min     money.Micros
	Max     money.Micros
	Claimed chain.Signature
}

func (b ArrivalBand) Contains(m money.Micros) bool { return m.Cmp(b.Min) >= 0 && m.Cmp(b.Max) <= 0 }

type ArrivalExpectation interface {
	BridgeBacksInFlight(ctx context.Context, cabal ids.CabalID) ([]ArrivalBand, error)
	ClaimArrival(ctx context.Context, band uuid.UUID, sig chain.Signature) (bool, error)
}
