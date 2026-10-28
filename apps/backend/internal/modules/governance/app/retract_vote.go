package app

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type RetractVote struct {
	ProposalID ids.ProposalID
	VoterID    ids.UserID
}

type RetractVoteResult struct {
	ProposalID ids.ProposalID
	Status     domain.Status
	Tally      Tally
}

type RetractVoteHandler struct {
	uow   *db.UnitOfWork
	clock clock.Clock
	hints Hints
}

func NewRetractVoteHandler(uow *db.UnitOfWork, c clock.Clock, hints Hints) *RetractVoteHandler {
	return &RetractVoteHandler{uow: uow, clock: c, hints: hints}
}

func (h *RetractVoteHandler) Handle(ctx context.Context, cmd RetractVote) (RetractVoteResult, error) {
	var out RetractVoteResult
	var cabal uuid.UUID
	err := h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		out, err = h.retract(ctx, tx, cmd, &cabal)
		return err
	})
	if err != nil {
		return RetractVoteResult{}, err
	}
	h.hints.ProposalUpdated(ctx, ids.CabalIDFrom(cabal), cmd.ProposalID)
	return out, nil
}

func (h *RetractVoteHandler) retract(
	ctx context.Context, tx db.Tx, cmd RetractVote, cabal *uuid.UUID,
) (RetractVoteResult, error) {
	const op = "governance.RetractVote"
	q := sqlc.New(tx.Queries())
	p, err := q.LockProposal(ctx, sqlc.LockProposalParams{ID: cmd.ProposalID.UUID(), VoterID: cmd.VoterID.UUID()})
	if err != nil {
		return RetractVoteResult{}, lookupFailed(err, op)
	}
	now := h.clock.Now()
	if err := mayVote(p, now, op); err != nil {
		return RetractVoteResult{}, err
	}
	*cabal = p.CabalID
	ballot := sqlc.LockBallotParams{ProposalID: p.ID, VoterID: cmd.VoterID.UUID()}
	held, err := q.LockBallot(ctx, ballot)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return RetractVoteResult{}, errs.New(errs.CodeVoteNotFound, op)
	case err != nil:
		return RetractVoteResult{}, errs.Wrap(err, errs.CodeInternal, op)
	case !domain.Undoable(held.FirstCastAt, held.Changed, now):
		return RetractVoteResult{}, errs.New(errs.CodeUndoExpired, op)
	}
	removed, err := q.DeleteBallot(ctx, sqlc.DeleteBallotParams(ballot))
	switch {
	case err != nil:
		return RetractVoteResult{}, errs.Wrap(err, errs.CodeInternal, op)
	case removed == 0:
		return RetractVoteResult{}, errs.New(errs.CodeUndoExpired, op)
	}
	n, err := q.CountBallots(ctx, p.ID)
	if err != nil {
		return RetractVoteResult{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	voters := int(n.Voters)
	return RetractVoteResult{
		ProposalID: cmd.ProposalID, Status: domain.StatusOpen,
		Tally: Tally{
			Yes: int(n.Yes), No: int(n.No), Voters: voters,
			Needed: domain.ThresholdRule(p.Threshold).Needed(voters),
		},
	}, nil
}
