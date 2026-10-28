package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Ballot = governanceport.Ballot

type MarketRef struct {
	Micros     money.Micros
	ObservedAt time.Time
}

type CallOpening struct {
	AssetID *uuid.UUID
	Ballots []Ballot
}

type CallDecision struct {
	Ballots []Ballot
	Ref     *MarketRef
}

type CallFill struct{ Price money.Micros }

type CallEvidence struct {
	ballots Ballots
	market  Market
	reads   *sqlc.Queries
	clock   clock.Clock
}

func NewCallEvidence(p Ports, db sqlc.DBTX, clk clock.Clock) CallEvidence {
	return CallEvidence{ballots: p.Ballots, market: p.Market, reads: sqlc.New(db), clock: clk}
}

func (c CallEvidence) Opening(ctx context.Context, e events.ProposalCreated) (CallOpening, error) {
	if _, ok := domain.ParseKind(e.Kind); !ok {
		return CallOpening{}, nil
	}
	asset, found, err := c.assetOfMint(ctx, string(e.Mint))
	if err != nil {
		return CallOpening{}, err
	}
	ballots, err := c.ballots.Ballots(ctx, ids.ProposalIDFrom(e.ProposalID))
	if err != nil {
		return CallOpening{}, err
	}
	opening := CallOpening{Ballots: ballots}
	if found {
		id := asset.ID.UUID()
		opening.AssetID = &id
	}
	return opening, nil
}

func (c CallEvidence) Passed(ctx context.Context, e events.ProposalPassed) (CallDecision, error) {
	if _, ok := domain.ParseKind(e.Kind); !ok {
		return CallDecision{}, nil
	}
	asset, found, err := c.assetOfMint(ctx, string(e.Mint))
	if err != nil {
		return CallDecision{}, err
	}
	var id *uuid.UUID
	if found {
		v := asset.ID.UUID()
		id = &v
	}
	return c.decision(ctx, e.ProposalID, id)
}

func (c CallEvidence) Failed(ctx context.Context, e events.ProposalFailed) (CallDecision, error) {
	return c.openedDecision(ctx, e.ProposalID)
}

func (c CallEvidence) Expired(ctx context.Context, e events.ProposalExpired) (CallDecision, error) {
	return c.openedDecision(ctx, e.ProposalID)
}

func (c CallEvidence) Fill(ctx context.Context, e events.TradeConfirmed) (CallFill, error) {
	kind, ok := domain.ParseKind(e.Action)
	if e.Source.Kind != sourceProposal || !ok {
		return CallFill{}, nil
	}
	mint := e.OutMint
	if kind == domain.KindSell {
		mint = e.InMint
	}
	asset, found, err := c.assetOfMint(ctx, string(mint))
	if err != nil {
		return CallFill{}, err
	}
	if !found {
		return CallFill{}, errs.New(
			errs.CodeAssetNotFound, "ranking.CallEvidence.Fill", slog.String("mint", string(mint)),
		)
	}
	price, err := domain.FillPrice(e, asset.Decimals)
	return CallFill{Price: price}, err
}

const sourceProposal = "proposal"

func (c CallEvidence) openedDecision(ctx context.Context, proposal uuid.UUID) (CallDecision, error) {
	assets, err := c.reads.CallAssetIDs(ctx, proposal)
	if err != nil {
		return CallDecision{}, errs.Wrap(err, errs.CodeDBUnavailable, "ranking.CallEvidence.openedDecision")
	}
	var asset *uuid.UUID
	for _, a := range assets {
		if a.Valid {
			id := uuid.UUID(a.Bytes)
			asset = &id
		}
	}
	return c.decision(ctx, proposal, asset)
}

func (c CallEvidence) decision(ctx context.Context, proposal uuid.UUID, asset *uuid.UUID) (CallDecision, error) {
	ballots, err := c.ballots.Ballots(ctx, ids.ProposalIDFrom(proposal))
	if err != nil {
		return CallDecision{}, err
	}
	decision := CallDecision{Ballots: ballots}
	if asset == nil {
		return decision, nil
	}
	id := market.AssetIDFrom(*asset)
	prices, err := c.market.PricesAsOf(ctx, []market.AssetID{id}, c.clock.Now())
	if err != nil {
		return CallDecision{}, err
	}
	if price, ok := prices[id]; ok {
		decision.Ref = &MarketRef{Micros: price.Micros, ObservedAt: price.ObservedAt}
	}
	return decision, nil
}

func (c CallEvidence) assetOfMint(ctx context.Context, mint string) (market.Asset, bool, error) {
	assets, err := c.market.AssetsByMints(ctx, []string{mint})
	if err != nil {
		return market.Asset{}, false, err
	}
	for _, a := range assets {
		if a.Mint.String() == mint {
			return a, true, nil
		}
	}
	return market.Asset{}, false, nil
}
