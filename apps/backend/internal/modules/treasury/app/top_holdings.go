package app

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type TopHoldings struct {
	q *sqlc.Queries
}

var _ port.TopHoldingsReader = (*TopHoldings)(nil)

func NewTopHoldings(db sqlc.DBTX) *TopHoldings { return &TopHoldings{q: sqlc.New(db)} }

func (r *TopHoldings) TopHoldings(
	ctx context.Context, cabalIDs []ids.CabalID,
) (map[ids.CabalID]chain.SolanaAddress, error) {
	if len(cabalIDs) == 0 {
		return map[ids.CabalID]chain.SolanaAddress{}, nil
	}
	raw := make([]uuid.UUID, len(cabalIDs))
	for i, id := range cabalIDs {
		raw[i] = id.UUID()
	}
	rows, err := r.q.TopHoldings(ctx, raw)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeDBUnavailable, "treasury.TopHoldings")
	}
	out := make(map[ids.CabalID]chain.SolanaAddress, len(rows))
	for _, row := range rows {
		out[ids.CabalIDFrom(row.CabalID)] = chain.SolanaAddress(row.Asset)
	}
	return out, nil
}
