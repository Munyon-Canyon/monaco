package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
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
	UndoUntil  *time.Time
	MyVote     MyVote
}

type CastVoteHandler struct {
	uow    *db.UnitOfWork
	cabals Cabals
	clock  clock.Clock
	hints  Hints
	assets Assets
	prices market.Prices
}

type candidate struct {
	mint  string
	price *money.Micros
}

func NewCastVoteHandler(uow *db.UnitOfWork, cabals Cabals, c clock.Clock, hints Hints) *CastVoteHandler {
	return &CastVoteHandler{uow: uow, cabals: cabals, clock: c, hints: hints}
}

func (h *CastVoteHandler) Priced(a Assets, p market.Prices) *CastVoteHandler {
	h.assets, h.prices = a, p
	return h
}

func (h *CastVoteHandler) banned(ctx context.Context, proposal ids.ProposalID) error {
	const op = "governance.CastVote"
	cabal, err := sqlc.New(h.uow.Reads()).CabalOfProposal(ctx, proposal.UUID())
	if err != nil {
		return lookupFailed(err, op)
	}
	return notBanned(ctx, h.cabals, ids.CabalIDFrom(cabal), op)
}

func (h *CastVoteHandler) candidatePrice(ctx context.Context, proposal ids.ProposalID) candidate {
	const op = "governance.CastVote"
	if h.assets == nil || h.prices == nil {
		return candidate{}
	}
	raw, err := sqlc.New(h.uow.Reads()).ProposalMint(ctx, proposal.UUID())
	if errors.Is(err, sql.ErrNoRows) {
		return candidate{}
	}
	if err != nil {
		return priceUnavailable(ctx, proposal, errs.Wrap(err, errs.CodeInternal, op))
	}
	mint, err := market.ParseMint(raw)
	if err != nil {
		return priceUnavailable(ctx, proposal, err)
	}
	asset, err := h.assets.AssetByMint(ctx, mint)
	if err != nil {
		return priceUnavailable(ctx, proposal, err)
	}
	prices, err := h.prices.PricesAsOf(ctx, []market.AssetID{asset.ID}, h.clock.Now())
	if err != nil {
		return priceUnavailable(ctx, proposal, err)
	}
	found, ok := prices[asset.ID]
	if !ok {
		return candidate{mint: raw}
	}
	price := found.Micros
	return candidate{mint: raw, price: &price}
}

func priceUnavailable(ctx context.Context, proposal ids.ProposalID, err error) candidate {
	observability.Degraded(ctx, observability.GovernancePassedPriceUnavailable,
		slog.String("proposal_id", proposal.UUID().String()), slog.String("code", string(errs.CodeOf(err))))
	return candidate{}
}

func (h *CastVoteHandler) Handle(ctx context.Context, cmd CastVote) (CastVoteResult, error) {
	var out CastVoteResult
	var cabal uuid.UUID
	banned := h.banned(ctx, cmd.ProposalID)
	priced := h.candidatePrice(ctx, cmd.ProposalID)
	err := h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		out, err = h.vote(ctx, tx, cmd, banned, priced, &cabal)
		return err
	})
	if err != nil {
		return CastVoteResult{}, err
	}
	h.hints.ProposalUpdated(ctx, ids.CabalIDFrom(cabal), cmd.ProposalID)
	return out, nil
}

func (h *CastVoteHandler) vote(
	ctx context.Context, tx db.Tx, cmd CastVote, banned error, priced candidate, cabal *uuid.UUID,
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
	n, order, err := castAndRank(ctx, q, sqlc.CastBallotParams{
		ProposalID: p.ID, VoterID: cmd.VoterID.UUID(), Choice: string(cmd.Choice), CastAt: now,
	})
	if err != nil {
		return CastVoteResult{}, err
	}
	if err := recordCast(ctx, tx, p, cmd, now); err != nil {
		return CastVoteResult{}, err
	}
	voters, yes, no := int(n.Voters), int(n.Yes), int(n.No)
	out := CastVoteResult{
		ProposalID: cmd.ProposalID, Status: domain.StatusOpen, MyBallot: cmd.Choice,
		Tally: Tally{Yes: yes, No: no, Voters: voters, Needed: rule.Needed(voters)},
	}
	verdict := domain.Tally(rule, voters, yes, no)
	out.MyVote = *myVoteOf(order, verdict != domain.Undecided)
	if verdict == domain.Undecided {
		out.UndoUntil = undoUntil(n.FirstCastAt, n.Changed)
		return out, nil
	}
	fin, err := closeOut(p, verdict, priced)
	if err != nil {
		return CastVoteResult{}, err
	}
	out.Status = fin.status
	transition := decidedTransition(p.ID, out.Status, cmd.VoterID.UUID(), now, fin.price, fin.hasPrice)
	if _, err := q.Transition(ctx, transition); err != nil {
		return CastVoteResult{}, err
	}
	if fin.event == nil {
		return out, nil
	}
	return out, tx.Events.Append(ctx, fin.event)
}

type closing struct {
	status   domain.Status
	event    events.Event
	price    int64
	hasPrice bool
}

func closeOut(p sqlc.LockProposalRow, verdict domain.Outcome, priced candidate) (closing, error) {
	if verdict == domain.Failed {
		return closing{
			status: domain.StatusFailed,
			event:  events.ProposalFailed{V: 1, ProposalID: p.ID, CabalID: p.CabalID, Kind: p.Kind},
		}, nil
	}
	if domain.Kind(p.Kind) == domain.KindRule {
		return closing{status: domain.StatusPassed}, nil
	}
	comparison, err := decodeComparison(p.PriceComparison)
	if err != nil {
		return closing{}, err
	}
	price, hasPrice := priced.storable(p.Mint.String)
	return closing{status: domain.StatusPassed, event: passed(p, comparison), price: price, hasPrice: hasPrice}, nil
}

func decidedTransition(
	id uuid.UUID, to domain.Status, voter uuid.UUID, at time.Time, price int64, hasPrice bool,
) sqlc.TransitionParams {
	t := sqlc.TransitionParams{
		ID: id, FromStatus: string(domain.StatusOpen), ToStatus: string(to), At: at, DecidedBy: voter.String(),
	}
	t.PassedPriceMicros.Int64, t.PassedPriceMicros.Valid = price, hasPrice
	return t
}

func (c candidate) storable(mint string) (int64, bool) {
	if c.price == nil || c.mint != mint {
		return 0, false
	}
	if n := c.price.Uint64(); n <= math.MaxInt64 {
		return int64(n), true
	}
	return 0, false
}

func castAndRank(
	ctx context.Context, q *sqlc.Queries, arg sqlc.CastBallotParams,
) (sqlc.CastBallotRow, sqlc.BallotOrderRow, error) {
	n, err := q.CastBallot(ctx, arg)
	if err != nil {
		return sqlc.CastBallotRow{}, sqlc.BallotOrderRow{}, err
	}
	order, err := q.BallotOrder(ctx, sqlc.BallotOrderParams{ProposalID: arg.ProposalID, VoterID: arg.VoterID})
	return n, order, err
}

func undoUntil(firstCastAt time.Time, changed bool) *time.Time {
	if changed {
		return nil
	}
	until := firstCastAt.Add(domain.UndoWindow).UTC()
	return &until
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

func recordCast(ctx context.Context, tx db.Tx, p sqlc.LockProposalRow, cmd CastVote, at time.Time) error {
	if err := appendVoteCast(ctx, tx, p.ID, p.CabalID, cmd, at); err != nil {
		return err
	}
	drop := sqlc.DeleteReminderParams{ProposalID: p.ID, UserID: cmd.VoterID.UUID()}
	if _, err := sqlc.New(tx.Queries()).DeleteReminder(ctx, drop); err != nil {
		return errs.Wrap(err, errs.CodeInternal, "governance.CastVote")
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
		V: 1, ProposalID: p.ID, CabalID: p.CabalID, ProposerID: p.ProposerID, Kind: p.Kind, Symbol: p.Symbol.String,
		Mint: chain.SolanaAddress(p.Mint.String), USDCMicros: money.MicrosFromUint64(unsigned(p.UsdcMicros.Int64)),
		TokenAmount: unsigned(p.TokenAmount.Int64), QuoteOutAmount: unsigned(p.QuoteOutAmount.Int64),
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
