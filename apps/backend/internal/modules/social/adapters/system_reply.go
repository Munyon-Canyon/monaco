package adapters

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Thread struct {
	Hints    app.Hints
	Assets   app.Assets
	Holdings app.Holdings
	Tallies  app.Tallies
}

type ThreadFill struct {
	Skip     bool
	Decimals uint8
	After    uint64
}

func (h Thread) FetchConfirmed(ctx context.Context, e events.TradeConfirmed) (ThreadFill, error) {
	if e.Source.Kind != proposalSource {
		return ThreadFill{Skip: true}, nil
	}
	posted, err := h.Holdings.Posted(ctx, e.SwapID)
	if err != nil {
		return ThreadFill{}, err
	}
	if !posted {
		return ThreadFill{}, errs.New(errs.CodeFeedItemPending, "social.Thread.FetchConfirmed")
	}
	mint, _ := tradeLeg(e)
	asset, err := h.Assets.AssetByMint(ctx, string(mint))
	if err != nil {
		return ThreadFill{}, err
	}
	after, err := h.Holdings.Holding(ctx, ids.CabalIDFrom(e.CabalID), mint)
	if err != nil {
		return ThreadFill{}, err
	}
	return ThreadFill{Decimals: asset.Decimals, After: after}, nil
}

func (h Thread) ApplyConfirmed(
	ctx context.Context, tx db.Tx, e events.TradeConfirmed, fill ThreadFill, _ time.Time,
) error {
	if fill.Skip {
		return nil
	}
	item, err := app.ProposalFeedItem(ctx, tx.Queries(), e.Source.ID)
	if err != nil {
		return err
	}
	_, filled := tradeLeg(e)
	side := feed.Action(e.Action)
	before := holdingBefore(side, fill.After, filled)
	reply := app.SystemReply{
		ItemID: item, Outcome: domain.OutcomeFilled, Side: side, Symbol: e.Symbol,
		After: unitsOf(fill.After, fill.Decimals), At: e.ConfirmedAt,
	}
	if before > 0 {
		reply.Before = unitsOf(before, fill.Decimals)
	}
	return h.write(ctx, tx, reply)
}

func holdingBefore(side feed.Action, after, filled uint64) uint64 {
	if side == feed.ActionSell {
		return after + filled
	}
	return max(after, filled) - filled
}

func unitsOf(v uint64, decimals uint8) *money.BaseUnits {
	units := money.NewBaseUnits(v, decimals)
	return &units
}

func (h Thread) FetchClosed(ctx context.Context, e events.ProposalFailed) (ClosedProposal, error) {
	return Feed{Tallies: h.Tallies}.FetchClosed(ctx, e.ProposalID)
}

func (h Thread) FetchExpired(ctx context.Context, e events.ProposalExpired) (ClosedProposal, error) {
	return Feed{Tallies: h.Tallies}.FetchClosed(ctx, e.ProposalID)
}

func (h Thread) ApplyFailed(
	ctx context.Context, tx db.Tx, e events.ProposalFailed, closed ClosedProposal, at time.Time,
) error {
	return h.applyClosed(ctx, tx, e.ProposalID, closed, at)
}

func (h Thread) ApplyExpired(
	ctx context.Context, tx db.Tx, e events.ProposalExpired, closed ClosedProposal, at time.Time,
) error {
	return h.applyClosed(ctx, tx, e.ProposalID, closed, at)
}

func (h Thread) applyClosed(
	ctx context.Context, tx db.Tx, proposalID uuid.UUID, closed ClosedProposal, at time.Time,
) error {
	if closed.Skip {
		return nil
	}
	source, err := sqlc.New(tx.Queries()).GetProposalFeedSource(ctx, proposalID)
	if err != nil {
		code := errs.CodeInternal
		if errors.Is(err, pgx.ErrNoRows) {
			code = errs.CodeFeedItemPending
		}
		return errs.Wrap(err, code, "social.Thread.applyClosed")
	}
	proposal, err := feed.ParsePayload(source.Payload)
	if err != nil {
		return err
	}
	return h.write(ctx, tx, app.SystemReply{
		ItemID: source.ID, Outcome: domain.OutcomeClosed, Side: proposal.Action, Symbol: source.Symbol.String, At: at,
	})
}

func (h Thread) TradeFailed(ctx context.Context, tx db.Tx, e events.TradeFailed, at time.Time) error {
	if e.Source.Kind != proposalSource {
		return nil
	}
	item, err := app.ProposalFeedItem(ctx, tx.Queries(), e.Source.ID)
	if err != nil {
		return err
	}
	return h.write(ctx, tx, app.SystemReply{
		ItemID: item, Outcome: domain.OutcomeTradeFailed, Side: feed.Action(e.Action), Symbol: e.Symbol, At: at,
	})
}

func (h Thread) write(ctx context.Context, tx db.Tx, reply app.SystemReply) error {
	if err := app.WriteSystemReply(ctx, tx, h.Hints, reply); err != nil {
		return err
	}
	faultpoint.Hit(ctx, faultpoint.BeforeCommit)
	return nil
}
