package app

import (
	"context"
	"database/sql"
	"errors"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Queries struct {
	q *sqlc.Queries
}

func NewQueries(db sqlc.DBTX) Queries { return Queries{q: sqlc.New(db)} }

func (s Queries) Status(ctx context.Context, id ids.ProposalID) (domain.Status, error) {
	const op = "governance.Status"
	raw, err := s.q.StatusByID(ctx, id.UUID())
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", errs.New(errs.CodeProposalNotFound, op)
	case err != nil:
		return "", errs.Wrap(err, errs.CodeInternal, op)
	}
	status, err := domain.ParseStatus(raw)
	if err != nil {
		return "", errs.Wrap(err, errs.CodeDecodeFailed, op)
	}
	return status, nil
}

func (s Queries) Proposer(ctx context.Context, id ids.ProposalID) (ids.UserID, error) {
	const op = "governance.Proposer"
	raw, err := s.q.ProposerOfProposal(ctx, id.UUID())
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return ids.UserID{}, errs.New(errs.CodeProposalNotFound, op)
	case err != nil:
		return ids.UserID{}, errs.Wrap(err, errs.CodeDBUnavailable, op)
	}
	return ids.UserIDFrom(raw), nil
}
