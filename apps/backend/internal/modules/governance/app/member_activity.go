package app

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

var _ port.MemberActivity = Queries{}

func (s Queries) VotesBy(
	ctx context.Context, user ids.UserID, after *port.Cursor, limit int,
) ([]port.CastVote, error) {
	const op = "governance.VotesBy"
	rowLimit, err := activityLimit(limit, op)
	if err != nil {
		return nil, err
	}
	params := sqlc.VotesByParams{VoterID: user.UUID(), RowLimit: rowLimit}
	if after != nil {
		params.HasCursor, params.CursorAt, params.CursorID = true, after.At, after.ID
	}
	rows, err := s.q.VotesBy(ctx, params)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	out := make([]port.CastVote, len(rows))
	for i, row := range rows {
		out[i] = port.CastVote{
			ProposalID: ids.ProposalIDFrom(row.ProposalID), CabalID: ids.CabalIDFrom(row.CabalID),
			Kind: row.Kind, Symbol: row.Symbol, Mint: row.Mint, USDCMicros: microsOf(row.UsdcMicros),
			Choice: row.Choice, CastAt: row.CastAt,
		}
	}
	return out, nil
}

func (s Queries) ProposalsBy(
	ctx context.Context, user ids.UserID, after *port.Cursor, limit int,
) ([]port.ProposedRow, error) {
	const op = "governance.ProposalsBy"
	rowLimit, err := activityLimit(limit, op)
	if err != nil {
		return nil, err
	}
	params := sqlc.ProposalsByParams{ProposerID: user.UUID(), RowLimit: rowLimit}
	if after != nil {
		params.HasCursor, params.CursorAt, params.CursorID = true, after.At, after.ID
	}
	rows, err := s.q.ProposalsBy(ctx, params)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	out := make([]port.ProposedRow, len(rows))
	for i, row := range rows {
		status, err := domain.ParseStatus(row.Status)
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeDecodeFailed, op)
		}
		out[i] = port.ProposedRow{
			ProposalID: ids.ProposalIDFrom(row.ID), CabalID: ids.CabalIDFrom(row.CabalID),
			Kind: row.Kind, Symbol: row.Symbol, Mint: row.Mint, USDCMicros: microsOf(row.UsdcMicros),
			Status: status, Yes: int(row.YesCount), No: int(row.NoCount), CreatedAt: row.CreatedAt,
		}
	}
	return out, nil
}

func (s Queries) ProposalSubjects(
	ctx context.Context, proposals []ids.ProposalID,
) (map[ids.ProposalID]port.Subject, error) {
	const op = "governance.ProposalSubjects"
	raw := make([]uuid.UUID, len(proposals))
	for i, id := range proposals {
		raw[i] = id.UUID()
	}
	rows, err := s.q.ProposalSubjects(ctx, raw)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	out := make(map[ids.ProposalID]port.Subject, len(rows))
	for _, row := range rows {
		out[ids.ProposalIDFrom(row.ID)] = port.Subject{
			CabalID: ids.CabalIDFrom(row.CabalID), Kind: row.Kind, Symbol: row.Symbol,
			USDCMicros: microsOf(row.UsdcMicros),
		}
	}
	return out, nil
}

func microsOf(v int64) *uint64 {
	if v < 0 {
		return nil
	}
	micros := uint64(v)
	return &micros
}

func activityLimit(limit int, op string) (int32, error) {
	if limit < 1 || limit > port.MemberActivityMaxLimit {
		return 0, errs.New(errs.CodeInvalidInput, op, slog.Int("limit", limit))
	}
	return int32(limit), nil
}
