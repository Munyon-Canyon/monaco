package app

import (
	"context"
	"log/slog"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

func (h *ExecuteTradeHandler) fallback(
	ctx context.Context, cmd ExecuteTrade, req SwapRequest, winner multichain.Listing,
) (ExecuteTrade, bool, error) {
	next, ok := nextSolana(cmd.Route, winner.ID)
	if !ok || cmd.Action != domain.ActionBuy || winner.Network != chain.NetworkSolana {
		return cmd, false, nil
	}
	payer, err := h.d.Layer.d.Signer.FeePayer()
	if err != nil {
		return cmd, false, err
	}
	_, err = h.d.Venue.Order(ctx, OrderSpec{
		Taker: req.TreasuryWallet.Address, Payer: payer, InMint: req.InMint, OutMint: req.OutMint,
		InAmount: req.InAmount, SlippageBps: req.SlippageBps,
	})
	if errs.CodeOf(err) != errs.CodeJupiterRejected {
		return cmd, false, nil
	}
	observability.Info(ctx, observability.TradingRouteFellBack,
		slog.String("proposal_id", cmd.ProposalID.String()), slog.String("from_listing", winner.ID.String()),
		slog.String("to_listing", next.String()), slog.GroupAttrs("detail", errs.Detail(err)...))
	route := *cmd.Route
	route.Winner = next
	cmd.Route = &route
	return cmd, true, nil
}

func nextSolana(c *multichain.Comparison, after multichain.ListingID) (multichain.ListingID, bool) {
	if c == nil {
		return multichain.ListingID{}, false
	}
	i := slices.IndexFunc(c.Ranked, func(q multichain.Quote) bool { return q.ListingID == after })
	if i < 0 {
		return multichain.ListingID{}, false
	}
	for _, q := range c.Ranked[i+1:] {
		if q.Network == chain.NetworkSolana {
			return q.ListingID, true
		}
	}
	return multichain.ListingID{}, false
}
