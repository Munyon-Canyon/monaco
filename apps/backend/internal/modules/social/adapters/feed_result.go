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
	if err := h.writeResult(ctx, tx, source, proposalID, facts, at); err != nil {
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
	ctx context.Context, tx db.Tx, source sqlc.GetProposalFeedSourceRow, proposalID uuid.UUID,
	merge func(*feed.Payload), at time.Time,
) error {
	payload, _, err := lockResult(ctx, tx, proposalID)
	if err != nil {
		return err
	}
	merge(&payload)
	return storeResult(ctx, tx, source, proposalID, payload, at)
}

func (h Feed) recordSwap(
	ctx context.Context, tx db.Tx, proposalID uuid.UUID, record func(*feed.Payload) error, at time.Time,
) error {
	const op = "social.Feed.recordSwap"
	source, err := sqlc.New(tx.Queries()).GetProposalFeedSource(ctx, proposalID)
	if errors.Is(err, pgx.ErrNoRows) {
		return errs.New(errs.CodeFeedItemPending, op)
	}
	if err != nil {
		return err
	}
	payload, found, err := lockResult(ctx, tx, proposalID)
	if err != nil {
		return err
	}
	if !found {
		return errs.New(errs.CodeFeedItemPending, op)
	}
	if err := record(&payload); err != nil {
		return err
	}
	if err := storeResult(ctx, tx, source, proposalID, payload, at); err != nil {
		return err
	}
	faultpoint.Hit(ctx, faultpoint.BeforeCommit)
	tx.AfterCommit(func(ctx context.Context) { h.Bus.PublishHint(ctx, "global.feed", nil) })
	return nil
}

func lockResult(ctx context.Context, tx db.Tx, proposalID uuid.UUID) (feed.Payload, bool, error) {
	raw, err := sqlc.New(tx.Queries()).LockFeedResult(ctx, proposalID)
	if errors.Is(err, pgx.ErrNoRows) {
		return feed.Payload{}, false, nil
	}
	if err != nil {
		return feed.Payload{}, false, err
	}
	payload, err := feed.ParsePayload(raw)
	return payload, err == nil, err
}

func storeResult(
	ctx context.Context, tx db.Tx, source sqlc.GetProposalFeedSourceRow, proposalID uuid.UUID,
	payload feed.Payload, at time.Time,
) error {
	state, _, _, _, err := feed.DeriveFill(payload)
	if err != nil {
		return err
	}
	payload.FillState = state
	return sqlc.New(tx.Queries()).UpsertFeedResult(ctx, sqlc.UpsertFeedResultParams{
		ID: eventID(ctx), ProposalID: proposalID, CabalID: source.CabalID, CabalName: source.CabalName,
		ActorID: source.ActorID, AssetID: source.AssetID, Symbol: source.Symbol,
		Title: feed.RenderTitle(feed.KindResult, payload), Body: pgtype.Text{
			String: feed.RenderDetail(feed.KindResult, payload), Valid: true,
		},
		Payload: payload.JSON(), Status: pgtype.Text{String: payload.Status, Valid: true}, At: at,
	})
}
