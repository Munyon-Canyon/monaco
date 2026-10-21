package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

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
	uow    *db.UnitOfWork
	cabals Cabals
	clock  clock.Clock
	hints  Hints
}

func NewCastVoteHandler(uow *db.UnitOfWork, cabals Cabals, c clock.Clock, hints Hints) *CastVoteHandler {
	return &CastVoteHandler{uow: uow, cabals: cabals, clock: c, hints: hints}
}

func (h *CastVoteHandler) banned(ctx context.Context, proposal ids.ProposalID) error {
	const op = "governance.CastVote"
	cabal, err := sqlc.New(h.uow.Reads()).CabalOfProposal(ctx, proposal.UUID())
	if err != nil {
		return lookupFailed(err, op)
	}
	return notBanned(ctx, h.cabals, ids.CabalIDFrom(cabal), op)
}

func (h *CastVoteHandler) Handle(ctx context.Context, cmd CastVote) (CastVoteResult, error) {
	var out CastVoteResult
	var cabal uuid.UUID
	banned := h.banned(ctx, cmd.ProposalID)
	err := h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		out, err = h.vote(ctx, tx, cmd, banned, &cabal)
		return err
	})
	if err != nil {
		return CastVoteResult{}, err
	}
	h.hints.ProposalUpdated(ctx, ids.CabalIDFrom(cabal), cmd.ProposalID)
	return out, nil
}

func (h *CastVoteHandler) vote(
	ctx context.Context, tx db.Tx, cmd CastVote, banned error, cabal *uuid.UUID,
) (CastVoteResult, error) {
	const op = "governance.CastVote"
	q := sqlc.New(tx.Queries())
	p, err := q.LockProposal(ctx, sqlc.LockProposalParams{ID: cmd.ProposalID.UUID(), VoterID: cmd.VoterID.UUID()})
	if err != nil {
		return CastVoteResult{}, lookupFailed(err, op)
	}
	now := h.clock.Now()
	if err := mayVote(p, now, op); err != nil {
		return CastVoteResult{}, err
	}
	*cabal = p.CabalID
	if banned != nil {
		return CastVoteResult{}, banned
	}
	rule := domain.ThresholdRule(p.Threshold)
	n, err := q.CastBallot(ctx, sqlc.CastBallotParams{
		ProposalID: p.ID, VoterID: cmd.VoterID.UUID(), Choice: string(cmd.Choice), CastAt: now,
	})
	if err != nil {
		return CastVoteResult{}, err
	}
	if err := appendVoteCast(ctx, tx, p.ID, p.CabalID, cmd, now); err != nil {
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
		comparison, err := decodeComparison(p.PriceComparison)
		if err != nil {
			return CastVoteResult{}, err
		}
		out.Status, decided = domain.StatusPassed, passed(p, comparison)
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

func mayVote(p sqlc.LockProposalRow, now time.Time, op string) error {
	switch {
	case p.Status != string(domain.StatusOpen):
		return errs.New(errs.CodeProposalClosed, op, slog.String("status", p.Status))
	case !now.Before(p.ExpiresAt):
		return errs.New(errs.CodeProposalClosed, op, slog.Time("expires_at", p.ExpiresAt))
	case !p.IsVoter:
		return errs.New(errs.CodeNotAVoter, op)
	}
	return nil
}

func appendVoteCast(
	ctx context.Context, tx db.Tx, proposal, cabal uuid.UUID, cmd CastVote, at time.Time,
) error {
	return tx.Events.Append(ctx, events.VoteCast{
		V: 1, ProposalID: proposal, CabalID: cabal, VoterID: cmd.VoterID.UUID(), Choice: string(cmd.Choice), CastAt: at,
	})
}

func passed(p sqlc.LockProposalRow, comparison *multichain.Comparison) events.ProposalPassed {
	return events.ProposalPassed{
		V: 1, ProposalID: p.ID, CabalID: p.CabalID, ProposerID: p.ProposerID, Kind: p.Kind, Symbol: p.Symbol,
		Mint: chain.SolanaAddress(p.Mint), USDCMicros: money.MicrosFromUint64(unsigned(p.UsdcMicros.Int64)),
		TokenAmount: unsigned(p.TokenAmount.Int64), QuoteOutAmount: unsigned(p.QuoteOutAmount),
		PriceComparison: comparison, Network: chain.Network(p.Network),
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
