package app

import (
	"context"
	"math/big"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	siblingMarginBps = 300
	siblingShareNano = 1_000_000_000
)

func solanaSibling(c multichain.Comparison) multichain.ListingID {
	for _, q := range c.Ranked {
		if q.Network == chain.NetworkSolana {
			return q.ListingID
		}
	}
	return multichain.ListingID{}
}

func quotedShares(q multichain.Quote, l multichain.Listing) uint64 {
	if q.GrossOutput == nil || q.GrossOutput.Sign() <= 0 || l.SharesPerToken == nil {
		return 0
	}
	n := new(big.Int).Mul(q.GrossOutput, l.SharesPerToken.Num())
	n.Mul(n, big.NewInt(siblingShareNano))
	d := new(
		big.Int,
	).Mul(l.SharesPerToken.Denom(), new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(l.Decimals)), nil))
	n.Quo(n, d)
	if !n.IsUint64() {
		return 0
	}
	return n.Uint64()
}

func (r *RouteExec) siblingQuote(ctx context.Context, b routedBuy) (multichain.Listing, multichain.Quote, bool, error) {
	sibling, err := r.d.Listings.Listing(ctx, b.sibling)
	var quote multichain.Quote
	if err == nil {
		quote, err = r.d.Quoter.Quote(ctx, sibling, multichain.SideBuy, b.landed)
	}
	if err != nil && errs.Retryable(errs.CodeOf(err)) {
		return sibling, quote, false, err
	}
	return sibling, quote, err == nil, nil
}

func (r *RouteExec) strayedFromSibling(ctx context.Context, b routedBuy, requote multichain.Quote) (bool, error) {
	if b.sibling.IsZero() {
		return false, nil
	}
	sibling, quote, found, err := r.siblingQuote(ctx, b)
	if err != nil || !found {
		return false, err
	}
	evm, solana := quotedShares(requote, b.listing), quotedShares(quote, sibling)
	if evm == 0 || solana == 0 {
		return false, nil
	}
	gap := max(evm, solana) - min(evm, solana)
	bps, err := money.MulDiv(gap, 10_000, solana)
	return bps > siblingMarginBps, err
}
