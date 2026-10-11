package port

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type DepositFees interface {
	EstimatedDepositFees(ctx context.Context) (map[chain.Network]money.Micros, error)
}
