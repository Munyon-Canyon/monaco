package adapters

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const ruleProposal = "rule"

type ClosedProposal struct {
	Tally app.Tally
	Skip  bool
}

func (h Feed) FetchClosed(ctx context.Context, proposalID uuid.UUID) (ClosedProposal, error) {
	const op = "social.Feed.FetchClosed"
	id := ids.ProposalIDFrom(proposalID)
	tallies, err := h.Tallies.Tallies(ctx, []ids.ProposalID{id})
	if err != nil {
		return ClosedProposal{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	tally, ok := tallies[id]
	if !ok {
		return ClosedProposal{}, errs.New(errs.CodeProposalNotFound, op)
	}
	return ClosedProposal{Tally: tally, Skip: tally.Kind == ruleProposal}, nil
}

func (h Feed) FetchPassed(ctx context.Context, e events.ProposalPassed) (ClosedProposal, error) {
	return h.FetchClosed(ctx, e.ProposalID)
}

func (h Feed) FetchFailed(ctx context.Context, e events.ProposalFailed) (ClosedProposal, error) {
	return h.FetchClosed(ctx, e.ProposalID)
}

func (h Feed) FetchExpired(ctx context.Context, e events.ProposalExpired) (ClosedProposal, error) {
	return h.FetchClosed(ctx, e.ProposalID)
}

func (h Feed) ApplyPassed(
	ctx context.Context, tx db.Tx, e events.ProposalPassed, closed ClosedProposal, at time.Time,
) error {
	return h.applyClosure(ctx, tx, e.ProposalID, feed.StatusPassed, feed.OutcomePassed, closed, at)
}

func (h Feed) ApplyFailed(
	ctx context.Context, tx db.Tx, e events.ProposalFailed, closed ClosedProposal, at time.Time,
) error {
	return h.applyClosure(ctx, tx, e.ProposalID, feed.StatusFailed, feed.OutcomeFailed, closed, at)
}

func (h Feed) ApplyExpired(
	ctx context.Context, tx db.Tx, e events.ProposalExpired, closed ClosedProposal, at time.Time,
) error {
	return h.applyClosure(ctx, tx, e.ProposalID, feed.StatusExpired, feed.OutcomeFailed, closed, at)
}

func (h Feed) applyClosure(
	ctx context.Context, tx db.Tx, proposalID uuid.UUID, status feed.ProposalStatus, outcome feed.Outcome,
	closed ClosedProposal, at time.Time,
) error {
	if closed.Skip {
		return nil
	}
	if _, err := h.moveStatus(ctx, tx, proposalID, status, "", at); err != nil {
		return err
	}
	return h.ApplyClosed(ctx, tx, proposalID, status, outcome, closed.Tally, at)
}

func (h Feed) ApplyClosed(
	ctx context.Context, tx db.Tx, proposalID uuid.UUID, status feed.ProposalStatus, outcome feed.Outcome,
	tally app.Tally, at time.Time,
) error {
	if tally.Kind == ruleProposal {
		return nil
	}
	source, err := sqlc.New(tx.Queries()).GetProposalFeedSource(ctx, proposalID)
	if errors.Is(err, pgx.ErrNoRows) {
		return errs.New(errs.CodeFeedItemPending, "social.Feed.ApplyClosed")
	}
	if err != nil {
		return err
	}
	proposal, err := feed.ParsePayload(source.Payload)
	if err != nil {
		return err
	}
	facts := closeFacts(proposalID, proposal, source.ActorID.Bytes, status, outcome, tally)
	if err := h.writeResult(ctx, tx, proposalID, facts, at); err != nil {
		return err
	}
	faultpoint.Hit(ctx, faultpoint.BeforeCommit)
	tx.AfterCommit(func(ctx context.Context) { h.Bus.PublishHint(ctx, "global.feed", nil) })
	return nil
}

func closeFacts(
	proposalID uuid.UUID, proposal feed.Payload, proposer uuid.UUID, status feed.ProposalStatus, outcome feed.Outcome,
	tally app.Tally,
) func(*feed.Payload) {
	amount := proposal.USDCMicros
	if amount.IsZero() {
		amount = proposal.QuoteMicros
	}
	return func(p *feed.Payload) {
		p.CabalName, p.ActorName, p.Symbol, p.AssetName = proposal.CabalName, proposal.ActorName, proposal.Symbol, proposal.AssetName
		p.Action, p.USDCMicros, p.ShareOfPotBps = proposal.Action, amount, proposal.ShareOfPotBps
		p.TokenAmount, p.TokenDecimals = proposal.TokenAmount, proposal.TokenDecimals
		p.AssetKind, p.ProposalID = proposal.AssetKind, proposalID
		p.Status, p.Outcome = string(status), outcome
		p.Yes, p.No, p.Voters, p.Needed = tally.Yes, tally.No, tally.Voters, tally.Needed
		p.Shape = feed.ResultShape(proposal.Action, outcome, proposal.AssetKind)
		p.CallerID, p.CallerName = uuid.Nil, ""
		if outcome == feed.OutcomePassed {
			p.CallerID, p.CallerName = proposer, proposal.ActorName
		}
	}
}

func (h Feed) writeResult(
	ctx context.Context, tx db.Tx, proposalID uuid.UUID, merge func(*feed.Payload), at time.Time,
) error {
	q := sqlc.New(tx.Queries())
	source, err := q.GetProposalFeedSource(ctx, proposalID)
	if errors.Is(err, pgx.ErrNoRows) {
		return errs.New(errs.CodeFeedItemPending, "social.Feed.writeResult")
	}
	if err != nil {
		return err
	}
	var payload feed.Payload
	raw, err := q.LockFeedResult(ctx, proposalID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return err
	default:
		if payload, err = feed.ParsePayload(raw); err != nil {
			return err
		}
	}
	merge(&payload)
	return q.UpsertFeedResult(ctx, sqlc.UpsertFeedResultParams{
		ID: eventID(ctx), ProposalID: proposalID, CabalID: source.CabalID, CabalName: source.CabalName,
		ActorID: source.ActorID, AssetID: source.AssetID, Symbol: source.Symbol,
		Title: feed.RenderTitle(feed.KindResult, payload), Body: pgtype.Text{
			String: feed.RenderDetail(feed.KindResult, payload), Valid: true,
		},
		Payload: payload.JSON(), Status: pgtype.Text{String: payload.Status, Valid: true}, At: at,
	})
}
