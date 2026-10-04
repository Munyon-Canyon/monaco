package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Thresholds interface {
	Threshold(ctx context.Context, cabal ids.CabalID) (domain.ThresholdRule, error)
}

type CastVote struct {
	ProposalID ids.ProposalID
	VoterID    ids.UserID
	Choice     domain.Choice
}

type Tally struct {
	Yes, No, Voters, Needed int
}

type CastVoteResult struct {
	ProposalID ids.ProposalID
	Status     domain.Status
	Tally      Tally
	MyBallot   domain.Choice
}

type CastVoteHandler struct {
	uow        *db.UnitOfWork
	reads      sqlc.DBTX
	clock      clock.Clock
	thresholds Thresholds
	hints      Hints
}

func NewCastVoteHandler(
	uow *db.UnitOfWork, reads sqlc.DBTX, c clock.Clock, t Thresholds, hints Hints,
) *CastVoteHandler {
	return &CastVoteHandler{uow: uow, reads: reads, clock: c, thresholds: t, hints: hints}
}

func (h *CastVoteHandler) Handle(ctx context.Context, cmd CastVote) (CastVoteResult, error) {
	const op = "governance.CastVote"
	cabal, err := sqlc.New(h.reads).CabalOfProposal(ctx, cmd.ProposalID.UUID())
	if err != nil {
		return CastVoteResult{}, lookupFailed(err, op)
	}
	rule, err := h.thresholds.Threshold(ctx, ids.CabalIDFrom(cabal))
	if err != nil {
		return CastVoteResult{}, err
	}
	var out CastVoteResult
	err = h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		out, err = h.vote(ctx, tx, rule, cmd)
		return err
	})
	if err != nil {
		return CastVoteResult{}, err
	}
	h.hints.ProposalUpdated(ctx, ids.CabalIDFrom(cabal), cmd.ProposalID)
	return out, nil
}

func (h *CastVoteHandler) vote(
	ctx context.Context, tx db.Tx, rule domain.ThresholdRule, cmd CastVote,
) (CastVoteResult, error) {
	const op = "governance.CastVote"
	q := sqlc.New(tx.Queries())
	p, err := q.LockProposal(ctx, sqlc.LockProposalParams{ID: cmd.ProposalID.UUID(), VoterID: cmd.VoterID.UUID()})
	switch {
	case err != nil:
		return CastVoteResult{}, lookupFailed(err, op)
	case p.Status != string(domain.StatusOpen):
		return CastVoteResult{}, errs.New(errs.CodeProposalClosed, op, slog.String("status", p.Status))
	case !p.IsVoter:
		return CastVoteResult{}, errs.New(errs.CodeNotAVoter, op)
	}
	now := h.clock.Now()
	n, err := q.CastBallot(ctx, sqlc.CastBallotParams{
		ProposalID: p.ID, VoterID: cmd.VoterID.UUID(), Choice: string(cmd.Choice), CastAt: now,
	})
	if err != nil {
		return CastVoteResult{}, err
	}
	voters, yes, no := int(n.Voters), int(n.Yes), int(n.No)
	out := CastVoteResult{
		ProposalID: cmd.ProposalID, Status: domain.StatusOpen, MyBallot: cmd.Choice,
		Tally: Tally{Yes: yes, No: no, Voters: voters, Needed: rule.Needed(voters)},
	}
	var decided events.Event
	switch domain.Tally(rule, voters, yes, no) {
	case domain.Undecided:
		return out, nil
	case domain.Passed:
		out.Status, decided = domain.StatusPassed, passed(p)
	case domain.Failed:
		out.Status, decided = domain.StatusFailed, events.ProposalFailed{V: 1, ProposalID: p.ID, CabalID: p.CabalID}
	}
	if _, err := q.Transition(ctx, sqlc.TransitionParams{
		ID: p.ID, FromStatus: string(domain.StatusOpen), ToStatus: string(out.Status), At: now,
	}); err != nil {
		return CastVoteResult{}, err
	}
	return out, tx.Events.Append(ctx, decided)
}

func passed(p sqlc.LockProposalRow) events.ProposalPassed {
	return events.ProposalPassed{
		V: 1, ProposalID: p.ID, CabalID: p.CabalID, ProposerID: p.ProposerID, Kind: p.Kind, Symbol: p.Symbol,
		Mint: chain.SolanaAddress(p.Mint), USDCMicros: money.MicrosFromUint64(unsigned(p.UsdcMicros.Int64)),
		TokenAmount: unsigned(p.TokenAmount.Int64), QuoteOutAmount: unsigned(p.QuoteOutAmount),
	}
}

func unsigned(v int64) uint64 {
	if v <= 0 {
		return 0
	}
	return uint64(v)
}

func lookupFailed(err error, op string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return errs.New(errs.CodeProposalNotFound, op)
	}
	return errs.Wrap(err, errs.CodeInternal, op)
}
