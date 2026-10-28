package app

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

var _ port.InviteFacts = Queries{}

func (s Queries) NewestOpenProposal(ctx context.Context, cabal ids.CabalID) (port.OpenProposal, bool, error) {
	const op = "governance.NewestOpenProposal"
	row, err := s.q.NewestOpenProposalForCabal(ctx, cabal.UUID())
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return port.OpenProposal{}, false, nil
	case err != nil:
		return port.OpenProposal{}, false, errs.Wrap(err, errs.CodeInternal, op)
	}
	return openProposalOf(row), true, nil
}

func (s Queries) MonthTurnout(
	ctx context.Context, cabal ids.CabalID, from time.Time,
) (voted, eligible int, err error) {
	row, err := s.q.CabalTurnoutSince(ctx, sqlc.CabalTurnoutSinceParams{CabalID: cabal.UUID(), Since: from})
	if err != nil {
		return 0, 0, errs.Wrap(err, errs.CodeInternal, "governance.MonthTurnout")
	}
	return int(row.VotedCount), int(row.EligibleCount), nil
}

func openProposalOf(row sqlc.NewestOpenProposalForCabalRow) port.OpenProposal {
	out := port.OpenProposal{
		ID: ids.ProposalIDFrom(row.ID), Kind: row.Kind, Symbol: row.Symbol, Mint: row.Mint,
		ProposerID: ids.UserIDFrom(row.ProposerID), Thesis: row.Thesis.String,
		Yes: row.YesCount, No: row.NoCount, Voted: row.VotedCount, Eligible: row.VoterCount,
		ExpiresAt: row.ExpiresAt.UTC(),
	}
	if row.UsdcMicros.Valid && row.UsdcMicros.Int64 > 0 {
		micros := uint64(row.UsdcMicros.Int64)
		out.USDCMicros = &micros
	}
	return out
}
