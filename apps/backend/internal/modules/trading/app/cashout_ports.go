package app

import (
	"context"
	"log/slog"
	"maps"
	"slices"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Positions interface {
	SolanaPositions(ctx context.Context, cabal ids.CabalID) ([]treasuryport.Position, error)
	EVMPositions(ctx context.Context, cabal ids.CabalID) ([]treasuryport.ListingPosition, error)
	EVMCashPositions(ctx context.Context, cabal ids.CabalID) (map[chain.Network]money.Micros, error)
	CashOutReservations(ctx context.Context) (map[ids.CabalID]money.Micros, error)
}

func evmListings(ctx context.Context, p Positions, cabal ids.CabalID) (map[string]uint64, error) {
	listings, err := p.EVMPositions(ctx, cabal)
	if err != nil {
		return nil, err
	}
	held := map[string]uint64{}
	for _, l := range listings {
		if l.Units == nil || !l.Units.IsUint64() {
			return nil, errs.New(errs.CodeDecodeFailed, "trading.evmListings", slog.String("asset", l.Key))
		}
		if l.Units.Sign() > 0 && isEVMAsset(l.Key) {
			held[l.Key] = l.Units.Uint64()
		}
	}
	return held, nil
}

func isEVMAsset(asset string) bool {
	network, _, found := strings.Cut(asset, ":")
	return found && chain.Network(network).IsEVM()
}

func evmCashNetwork(asset string) (chain.Network, bool) {
	network, address, found := strings.Cut(asset, ":")
	n := chain.Network(network)
	return n, found && n.IsEVM() && strings.EqualFold(address, n.Cash().Address)
}

type LedgerHoldings struct {
	Ledger  Positions
	Catalog Catalog
	USDC    chain.Mint
}

func (h LedgerHoldings) evmHeld(ctx context.Context, cabal ids.CabalID) ([]Holding, error) {
	evm, err := evmListings(ctx, h.Ledger, cabal)
	if err != nil {
		return nil, err
	}
	cash, err := h.Ledger.EVMCashPositions(ctx, cabal)
	if err != nil {
		return nil, err
	}
	held := make([]Holding, 0, len(evm)+len(cash))
	for _, asset := range slices.Sorted(maps.Keys(evm)) {
		held = append(held, Holding{Mint: chain.Mint{Address: chain.SolanaAddress(asset)}, Units: evm[asset]})
	}
	for _, network := range slices.Sorted(maps.Keys(cash)) {
		if cash[network].IsZero() || !network.IsEVM() {
			continue
		}
		held = append(held, Holding{
			Mint: chain.Mint{
				Address:  chain.SolanaAddress(evmAsset(network, network.Cash().Address)),
				Decimals: network.Cash().Decimals,
			},
			Symbol: network.Cash().Symbol,
			Units:  cash[network].Uint64(),
		})
	}
	return held, nil
}

func (h LedgerHoldings) Positions(ctx context.Context, cabal ids.CabalID) ([]Holding, error) {
	positions, err := h.Ledger.SolanaPositions(ctx, cabal)
	if err != nil {
		return nil, err
	}
	held, err := h.evmHeld(ctx, cabal)
	if err != nil {
		return nil, err
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
