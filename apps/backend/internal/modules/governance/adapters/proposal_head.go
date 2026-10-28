package adapters

import (
	"context"
	"database/sql"
	"errors"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type ProposalHeads struct{ q *sqlc.Queries }

var _ port.ProposalHeads = ProposalHeads{}

func NewProposalHeads(db sqlc.DBTX) ProposalHeads { return ProposalHeads{q: sqlc.New(db)} }

func (h ProposalHeads) ProposalHead(ctx context.Context, id ids.ProposalID) (port.ProposalHead, error) {
	const op = "governance.ProposalHead"
	row, err := h.q.GetProposalHead(ctx, id.UUID())
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return port.ProposalHead{}, errs.Wrap(err, errs.CodeProposalNotFound, op)
	case err != nil:
		return port.ProposalHead{}, errs.Wrap(err, errs.CodeDBUnavailable, op)
	}
	return port.ProposalHead{Kind: row.Kind, Symbol: row.Symbol, Thesis: row.Thesis}, nil
}
