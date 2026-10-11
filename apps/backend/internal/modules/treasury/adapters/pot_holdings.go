package adapters

import (
	"context"
	"math/big"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

type stockGroup struct {
	index    int
	shares   *big.Rat
	listings []port.HoldingListing
}

func (q *Queries) mergeEVM(
	ctx context.Context, pot *potBuilder, solana []port.Position, assets map[chain.SolanaAddress]app.Asset,
	split chainRows,
) error {
	solanaKeys := make([]string, len(solana))
	for i, p := range solana {
		solanaKeys[i] = solanaListingKey(p.Mint)
	}
	held, quotes, asOf, err := q.valueEVM(ctx, split.evm, solanaKeys...)
	if err != nil {
		return err
	}
	if asOf.Before(pot.PricesAsOf) {
		pot.PricesAsOf = asOf
	}
	groups := map[string]*stockGroup{}
	for _, h := range held {
		key := groupKey(h)
		g, ok := groups[key]
		if !ok {
			g = q.newGroup(pot, h, solana, assets, quotes)
			groups[key] = g
		}
		if err := g.add(&pot.Holdings[g.index], h); err != nil {
			return errs.Wrap(err, errs.CodeOf(err), "treasury.Queries.mergeEVM")
		}
	}
	return nil
}

func (q *Queries) newGroup(
	pot *potBuilder, h evmHolding, solana []port.Position, assets map[chain.SolanaAddress]app.Asset,
	quotes map[string]app.ListingQuote,
) *stockGroup {
	for i, p := range solana {
		if string(p.Mint) != groupKey(h) {
			continue
		}
		g := &stockGroup{index: i, shares: q.solanaShares(p, assets[p.Mint])}
		if quote, ok := quotes[solanaListingKey(p.Mint)]; ok {
			g.listings = []port.HoldingListing{{
				ListingID: quote.ListingID, Issuer: quote.Issuer, Network: chain.NetworkSolana,
				TokenAmount: new(big.Int).SetUint64(p.Units.Uint64()), ValueMicros: pot.Holdings[i].ValueMicros,
			}}
		}
		return g
	}
	pot.Holdings = append(pot.Holdings, evmOnlyHolding(h))
	return &stockGroup{index: len(pot.Holdings) - 1, shares: new(big.Rat)}
}

func (g *stockGroup) add(holding *port.Holding, h evmHolding) error {
	shares := new(big.Rat).SetInt(h.units)
	shares.Mul(shares, h.quote.SharesPerToken)
	shares.Quo(shares, new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(h.quote.Decimals)), nil)))
	g.shares.Add(g.shares, shares)
	g.listings = append(g.listings, port.HoldingListing{
		ListingID: h.quote.ListingID, Issuer: h.quote.Issuer, Network: h.quote.Network,
		TokenAmount: h.units, ValueMicros: h.value,
	})
	var err error
	if holding.ValueMicros, err = holding.ValueMicros.Add(h.value); err != nil {
		return err
	}
	if holding.CostBasisMicros, err = holding.CostBasisMicros.Add(h.cost); err != nil {
		return err
	}
	if holding.PnLMicros, err = signedSub(
		microsInt(holding.ValueMicros),
		microsInt(holding.CostBasisMicros),
	); err != nil {
		return err
	}
	holding.Units, holding.Listings = formatShares(g.shares), g.listings
	return nil
}

func (q *Queries) solanaShares(p port.Position, asset app.Asset) *big.Rat {
	num, den := asset.UIMultiplierAt(q.clock.Now())
	shares := new(big.Rat).SetFrac(new(big.Int).SetUint64(p.Units.Uint64()), big.NewInt(1))
	shares.Mul(shares, big.NewRat(num, den))
	return shares.Quo(
		shares,
		new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(p.Units.Decimals())), nil)),
	)
}

func evmOnlyHolding(h evmHolding) port.Holding {
	holding := port.Holding{
		Symbol:      h.quote.Symbol,
		DisplayName: h.quote.Symbol,
		Kind:        "equity",
		PriceMicros: h.quote.Price.Micros,
	}
	if a := h.quote.Asset; a != nil {
		holding.Symbol, holding.DisplayName, holding.Kind = a.Symbol, a.DisplayName, a.Kind
	}
	return holding
}

func formatShares(shares *big.Rat) string {
	scaled := new(big.Rat).Mul(shares, big.NewRat(10_000, 1))
	floor := new(big.Int).Quo(scaled.Num(), scaled.Denom())
	return new(big.Rat).SetFrac(floor, big.NewInt(10_000)).FloatString(unitsDisplayDecimals)
}

func solanaListingKey(mint chain.SolanaAddress) string {
	return string(chain.NetworkSolana) + ":" + string(mint)
}

func groupKey(h evmHolding) string {
	if h.quote.Asset != nil && h.quote.Asset.Mint != "" {
		return h.quote.Asset.Mint
	}
	return h.key
}

func (q *Queries) addOffSolana(
	ctx context.Context, pot *potBuilder, solana []port.Position, assets map[chain.SolanaAddress]app.Asset,
	split chainRows, transitRaw string,
) error {
	const op = "treasury.Queries.addOffSolana"
	transit, err := micros(transitRaw)
	if err != nil {
		return err
	}
	if pot.CashMicros, err = pot.CashMicros.Add(split.evmCash); err != nil {
		return errs.Wrap(err, errs.CodeOf(err), op)
	}
	if pot.CashMicros, err = pot.CashMicros.Add(transit); err != nil {
		return errs.Wrap(err, errs.CodeOf(err), op)
	}
	return q.mergeEVM(ctx, pot, solana, assets, split)
}
