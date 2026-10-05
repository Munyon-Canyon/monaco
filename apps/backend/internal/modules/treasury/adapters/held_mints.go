package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

type HeldMints struct{ q *sqlc.Queries }

var _ port.HeldMints = HeldMints{}

func NewHeldMints(db sqlc.DBTX) HeldMints { return HeldMints{q: sqlc.New(db)} }

func (h HeldMints) HeldMints(ctx context.Context) ([]chain.SolanaAddress, error) {
	rows, err := h.q.HeldMints(ctx)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "treasury.HeldMints")
	}
	out := make([]chain.SolanaAddress, len(rows))
	for i, mint := range rows {
		out[i] = chain.SolanaAddress(mint)
	}
	return out, nil
}
