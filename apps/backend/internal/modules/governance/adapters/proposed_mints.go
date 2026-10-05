package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

type ProposedMints struct{ q *sqlc.Queries }

var _ port.ProposedMints = ProposedMints{}

func NewProposedMints(db sqlc.DBTX) ProposedMints { return ProposedMints{q: sqlc.New(db)} }

func (p ProposedMints) ProposedMints(ctx context.Context) ([]chain.SolanaAddress, error) {
	rows, err := p.q.ProposedMints(ctx)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "governance.ProposedMints")
	}
	out := make([]chain.SolanaAddress, len(rows))
	for i, mint := range rows {
		out[i] = chain.SolanaAddress(mint)
	}
	return out, nil
}
