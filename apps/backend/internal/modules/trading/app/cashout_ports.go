package app

import (
	"context"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Positions interface {
	SolanaPositions(ctx context.Context, cabal ids.CabalID) ([]treasuryport.Position, error)
	CabalPositionsAt(ctx context.Context, t time.Time) ([]treasuryport.CabalPositions, error)
}

func evmListings(ctx context.Context, p Positions, at time.Time, cabal ids.CabalID) (map[string]uint64, error) {
	all, err := p.CabalPositionsAt(ctx, at)
	if err != nil {
		return nil, err
	}
	held := map[string]uint64{}
	for _, c := range all {
		if c.CabalID != cabal {
			continue
		}
		for _, l := range c.Listings {
			if l.Units == nil || !l.Units.IsUint64() {
				return nil, errs.New(errs.CodeDecodeFailed, "trading.evmListings", slog.String("asset", l.Key))
			}
			if l.Units.Sign() > 0 && isEVMAsset(l.Key) {
				held[l.Key] = l.Units.Uint64()
			}
		}
	}
	return held, nil
}

func isEVMAsset(asset string) bool {
	network, _, found := strings.Cut(asset, ":")
	return found && chain.Network(network).IsEVM()
}

type LedgerHoldings struct {
	Ledger  Positions
	Catalog Catalog
	USDC    chain.Mint
	Clock   clock.Clock
}

func (h LedgerHoldings) Positions(ctx context.Context, cabal ids.CabalID) ([]Holding, error) {
	positions, err := h.Ledger.SolanaPositions(ctx, cabal)
	if err != nil {
		return nil, err
	}
	evm, err := evmListings(ctx, h.Ledger, h.Clock.Now(), cabal)
	if err != nil {
		return nil, err
	}
	held := make([]Holding, 0, len(positions)+len(evm))
	for _, asset := range slices.Sorted(maps.Keys(evm)) {
		held = append(held, Holding{Mint: chain.Mint{Address: chain.SolanaAddress(asset)}, Units: evm[asset]})
	}
	for _, p := range positions {
		if p.Mint == h.USDC.Address || p.Units.IsZero() || isEVMAsset(string(p.Mint)) {
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

type EVMSale struct {
	Listing        multichain.Listing
	ProceedsMicros uint64
}

type EVMSales interface {
	Sale(ctx context.Context, asset string, units uint64) (EVMSale, error)
}

type UnwiredEVMSales struct{}

func (UnwiredEVMSales) Sale(context.Context, string, uint64) (EVMSale, error) {
	return EVMSale{}, notImplemented("EVMSales.Sale", "#4969")
}
