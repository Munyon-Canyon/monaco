package port

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type ArrivalExpectation interface {
	BridgeBackInFlight(ctx context.Context, cabal ids.CabalID) (money.Micros, error)
}
