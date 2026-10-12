package adapters

import (
	"context"
	"strconv"

	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	wire "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/governanceapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func (h HTTP) GetCabalProposalPriceComparison(
	ctx context.Context, req api.GetCabalProposalPriceComparisonRequestObject,
) (api.GetCabalProposalPriceComparisonResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	live, err := h.Comparisons.Get(ctx, app.GetProposalComparison{
		CabalID: ids.CabalIDFrom(req.Id), ProposalID: ids.ProposalIDFrom(req.ProposalId), Caller: user,
	})
	if err != nil {
		return nil, err
	}
	quote, err := wireInt(live.QuoteOut, "quote_out_amount")
	if err != nil {
		return nil, err
	}
	return api.GetCabalProposalPriceComparison200JSONResponse{
		QuoteOutAmount: quote, QuotedAt: live.QuotedAt, PriceComparison: wireComparison(live.Comparison),
	}, nil
}

func wireComparison(c *multichain.Comparison) *wire.PriceComparison {
	if c == nil {
		return nil
	}
	out := &wire.PriceComparison{
		WinnerListingId: c.Winner.UUID(), Quotes: make([]wire.RouteQuote, len(c.Ranked)),
		Unavailable: make([]wire.UnavailableVenue, len(c.Unavailable)),
	}
	for i, u := range c.Unavailable {
		out.Unavailable[i] = wire.UnavailableVenue{
			ListingId: u.ListingID.UUID(), Network: wire.Network(u.Network),
			Venue: wire.UnavailableVenueVenue(u.Venue), Reason: wire.UnavailableVenueReason(u.Reason),
		}
		if u.Detail != "" {
			out.Unavailable[i].ReasonDetail = &u.Detail
		}
	}
	for i, q := range c.Ranked {
		costs := make([]wire.RouteCost, len(q.Costs))
		for j, cost := range q.Costs {
			costs[j] = wire.RouteCost{Kind: wire.RouteCostKind(cost.Kind), Micros: cost.Micros.String()}
		}
		out.Quotes[i] = wire.RouteQuote{
			ListingId: q.ListingID.UUID(), Issuer: q.Issuer, Network: wire.Network(q.Network),
			Venue: wire.RouteQuoteVenue(q.Venue), NetShareNanos: strconv.FormatUint(uint64(q.NetShares), 10),
			NetMicros: q.NetMicros.String(), Costs: costs, QuotedAt: q.QuotedAt, ExpiresAt: q.ExpiresAt,
		}
	}
	return out
}
