package adapters

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type VoteStates struct{ q *sqlc.Queries }

var _ port.VoteStates = VoteStates{}

func NewVoteStates(db sqlc.DBTX) VoteStates { return VoteStates{q: sqlc.New(db)} }

func (v VoteStates) VoteState(ctx context.Context, id ids.ProposalID) (port.VoteState, error) {
	const op = "governance.VoteState"
	row, err := v.q.GetVoteState(ctx, id.UUID())
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return port.VoteState{}, errs.Wrap(err, errs.CodeProposalNotFound, op)
	case err != nil:
		return port.VoteState{}, errs.Wrap(err, errs.CodeDBUnavailable, op)
	}
	status, err := domain.ParseStatus(row.Status)
	if err != nil {
		return port.VoteState{}, errs.Wrap(err, errs.CodeDecodeFailed, op)
	}
	eligible := int(row.Eligible)
	return port.VoteState{
		CabalID: ids.CabalIDFrom(row.CabalID), Kind: row.Kind, Symbol: row.Symbol, Mint: row.Mint,
		USDCMicros: money.MicrosFromUint64(uint64(max(row.UsdcMicros, 0))), Status: status,
		Yes: int(row.Yes), No: int(row.No), Eligible: eligible,
		Need:      domain.ThresholdRule(row.Threshold).Needed(eligible),
		CreatedAt: row.CreatedAt.UTC(), ExpiresAt: row.ExpiresAt.UTC(),
	}, nil
}

func (v VoteStates) DueForReminder(
	ctx context.Context, now time.Time, after port.ReminderCursor, limit int,
) ([]port.DueReminder, error) {
	const op = "governance.DueForReminder"
	rows, err := v.q.DueForReminder(ctx, sqlc.DueForReminderParams{
		AfterDueAt: after.DueAt, AfterID: after.ProposalID.UUID(), Now: now,
		Batch: int32(min(max(limit, 0), math.MaxInt32)),
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeDBUnavailable, op)
	}
	out := make([]port.DueReminder, len(rows))
	for i, row := range rows {
		out[i] = port.DueReminder{ProposalID: ids.ProposalIDFrom(row.ID), DueAt: row.DueAt.UTC()}
	}
	return out, nil
}

func (v VoteStates) NonVoters(ctx context.Context, id ids.ProposalID) ([]ids.UserID, error) {
	rows, err := v.q.NonVotersOfProposal(ctx, id.UUID())
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeDBUnavailable, "governance.NonVoters")
	}
	out := make([]ids.UserID, len(rows))
	for i, row := range rows {
		out[i] = ids.UserIDFrom(row)
	}
	return out, nil
}
