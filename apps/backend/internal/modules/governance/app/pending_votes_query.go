package app

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type PendingVote struct {
	ProposalID ids.ProposalID
	CabalID    ids.CabalID
	Kind       domain.Kind
	Symbol     string
	ExpiresAt  time.Time
}

func (r *ProposalReads) PendingVotes(ctx context.Context, voter ids.UserID) ([]PendingVote, error) {
	rows, err := r.q.PendingVotes(ctx, voter.UUID())
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "governance.PendingVotes")
	}
	out := make([]PendingVote, len(rows))
	for i, row := range rows {
		kind, err := domain.ParseKind(row.Kind)
		if err != nil {
			return nil, err
		}
		out[i] = PendingVote{
			ProposalID: ids.ProposalIDFrom(row.ID), CabalID: ids.CabalIDFrom(row.CabalID), Kind: kind,
			Symbol: row.Symbol, ExpiresAt: row.ExpiresAt,
		}
	}
	return out, nil
}
