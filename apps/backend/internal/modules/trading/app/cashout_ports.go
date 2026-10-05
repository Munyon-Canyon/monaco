package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Positions interface {
	Positions(ctx context.Context, cabal ids.CabalID) ([]treasuryport.Position, error)
}

type LedgerHoldings struct {
	Ledger  Positions
	Catalog Catalog
	USDC    chain.Mint
}

func (h LedgerHoldings) Positions(ctx context.Context, cabal ids.CabalID) ([]Holding, error) {
	positions, err := h.Ledger.Positions(ctx, cabal)
	if err != nil {
		return nil, err
	}
	held := make([]Holding, 0, len(positions))
	for _, p := range positions {
		if p.Mint == h.USDC.Address || p.Units.IsZero() {
			continue
		}
		mint, err := market.ParseMint(string(p.Mint))
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeDecodeFailed, "trading.LedgerHoldings")
		}
		asset, err := h.Catalog.AssetByMint(ctx, mint)
		if err != nil {
			return nil, err
		}
		held = append(held, Holding{
			Mint:   chain.Mint{Address: p.Mint, Decimals: asset.Decimals},
			Symbol: asset.Symbol,
			Units:  p.Units.Uint64(),
		})
	}
	return held, nil
}

type CabalWallets struct{ Cabals Cabals }

func (w CabalWallets) TreasuryWallet(ctx context.Context, cabal ids.CabalID) (TreasuryWallet, error) {
	tw, err := w.Cabals.TreasuryWallet(ctx, cabal)
	if err != nil {
		return TreasuryWallet{}, err
	}
	return TreasuryWallet{PrivyWalletID: tw.PrivyWalletID, Address: tw.Address}, nil
}
