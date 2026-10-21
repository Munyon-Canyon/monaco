package app

import (
	"context"
	"log/slog"
	"math/big"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	saleProbeMicros = 1_000_000
	saleQuoteTTL    = time.Minute
)

type EVMHoldings interface {
	EVMPositions(ctx context.Context, id ids.CabalID) ([]treasuryport.ListingPosition, error)
}

type Listings interface {
	ListingByAsset(ctx context.Context, asset string) (multichain.Listing, error)
}

type evmHeld struct {
	listing multichain.Listing
	units   *big.Int
}

type sellTarget struct {
	network   chain.Network
	positions []treasuryport.Position
	held      evmHeld
}

func (h *ProposeTradeHandler) sellTarget(
	ctx context.Context, cabal ids.CabalID, asset market.Asset, trade domain.Trade,
) (sellTarget, error) {
	positions, err := h.ports.Treasury.SolanaPositions(ctx, cabal)
	if err != nil {
		return sellTarget{}, err
	}
	held, err := h.heldEVM(ctx, cabal, asset)
	if err != nil {
		return sellTarget{}, err
	}
	network, err := sellNetwork(trade.Network, onSolana(positions, asset), held)
	return sellTarget{network: network, positions: positions, held: held[network]}, err
}

func onSolana(positions []treasuryport.Position, asset market.Asset) bool {
	for _, p := range positions {
		if p.Mint == asset.Mint.Address() && !p.Units.IsZero() {
			return true
		}
	}
	return false
}

func (h *ProposeTradeHandler) heldEVM(
	ctx context.Context, cabal ids.CabalID, asset market.Asset,
) (map[chain.Network]evmHeld, error) {
	const op = "governance.heldEVM"
	reader, ok := h.ports.Treasury.(EVMHoldings)
	if !ok || h.ports.Listings == nil {
		return map[chain.Network]evmHeld{}, nil
	}
	positions, err := reader.EVMPositions(ctx, cabal)
	if err != nil {
		return nil, err
	}
	held := map[chain.Network]evmHeld{}
	for _, p := range positions {
		if p.Units == nil || p.Units.Sign() <= 0 {
			continue
		}
		l, err := h.ports.Listings.ListingByAsset(ctx, p.Key)
		if code := errs.CodeOf(err); code == errs.CodeAssetNotFound || code == errs.CodeAssetUntradable {
			continue
		} else if err != nil {
			return nil, err
		}
		if l.StockID.String() != asset.ID.String() {
			continue
		}
		if _, twice := held[l.Network]; twice {
			return nil, errs.New(errs.CodeInvalidNetwork, op, slog.String("network", string(l.Network)))
		}
		held[l.Network] = evmHeld{listing: l, units: p.Units}
	}
	return held, nil
}

func sellNetwork(pin chain.Network, solana bool, held map[chain.Network]evmHeld) (chain.Network, error) {
	const op = "governance.sellNetwork"
	if pin != "" {
		if _, ok := held[pin]; pin != chain.NetworkSolana && (!pin.Trades() || !ok) {
			return "", errs.New(errs.CodeInvalidNetwork, op, slog.String("network", string(pin)))
		}
		return pin, nil
	}
	if solana || len(held) == 0 {
		return "", nil
	}
	for network := range held {
		if len(held) == 1 {
			return network, nil
		}
	}
	return "", errs.New(errs.CodeInvalidNetwork, op, slog.String("reason", "pin_required"))
}

func (h *ProposeTradeHandler) sellFunds(
	ctx context.Context, cabal ids.CabalID, asset market.Asset, trade domain.Trade, t sellTarget,
) error {
	if t.network.IsEVM() {
		return evmHolds(t.held, trade)
	}
	if err := ledgerHolds(t.positions, asset, trade); err != nil {
		return err
	}
	return walletHolds(ctx, h.ports, cabal, asset, trade)
}

func evmHolds(held evmHeld, trade domain.Trade) error {
	if held.units.Cmp(new(big.Int).SetUint64(trade.TokenAmount)) < 0 {
		return errs.New(errs.CodeCabalSharesShort, "governance.funds",
			slog.String("symbol", held.listing.Symbol), slog.String("network", string(held.listing.Network)))
	}
	return nil
}

func ledgerHolds(positions []treasuryport.Position, asset market.Asset, trade domain.Trade) error {
	const op = "governance.funds"
	want := money.NewBaseUnits(trade.TokenAmount, asset.Decimals)
	for _, p := range positions {
		if p.Mint != asset.Mint.Address() {
			continue
		}
		short, err := p.Units.Cmp(want)
		if err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		if short >= 0 {
			return nil
		}
	}
	return errs.New(errs.CodeCabalSharesShort, op, slog.String("symbol", asset.Symbol))
}

func walletHolds(
	ctx context.Context, p TradePorts, cabal ids.CabalID, asset market.Asset, trade domain.Trade,
) error {
	wallet, err := p.Cabals.TreasuryWallet(ctx, cabal)
	if err != nil {
		return err
	}
	mint := chain.Mint{Address: asset.Mint.Address(), Decimals: asset.Decimals}
	have, err := p.Balances.TokenBalance(ctx, wallet.Address, mint)
	if err != nil {
		return err
	}
	if have.Uint64() < trade.TokenAmount {
		return errs.New(errs.CodeCabalSharesShort, "governance.funds", slog.String("symbol", asset.Symbol))
	}
	return nil
}

func (h *ProposeTradeHandler) sellRoute(
	ctx context.Context, asset market.Asset, trade domain.Trade, t sellTarget,
) (assessment, error) {
	if !t.network.IsEVM() {
		routed, _, err := h.route(ctx, asset, trade)
		return assessment{asset: asset, decimals: asset.Decimals, routed: routed, network: trade.Network}, err
	}
	l := t.held.listing
	if !l.Enabled || !l.Network.Trades() {
		return assessment{}, errs.New(errs.CodeAssetUntradable, "governance.sellRoute", slog.String("symbol", l.Symbol))
	}
	proceeds, err := h.saleProceeds(ctx, l, trade.TokenAmount)
	if err != nil {
		return assessment{}, err
	}
	return assessment{
		asset: asset, decimals: l.Decimals, routed: proceeds, network: l.Network,
		comparison: saleComparison(l, proceeds, trade.TokenAmount, h.clock.Now()),
	}, nil
}

func (h *ProposeTradeHandler) saleProceeds(ctx context.Context, l multichain.Listing, units uint64) (uint64, error) {
	const op = "governance.saleProceeds"
	if h.ports.Quoter == nil || units == 0 {
		return 0, errs.New(errs.CodeNoRoute, op, slog.String("listing_id", l.ID.String()))
	}
	estimate, err := h.quoteSale(ctx, l, saleProbeMicros, units)
	if err != nil {
		return 0, err
	}
	refined, err := h.quoteSale(ctx, l, estimate, units)
	if err != nil {
		return 0, err
	}
	if proceeds := min(estimate, refined); proceeds > 0 {
		return proceeds, nil
	}
	return 0, errs.New(errs.CodeNoRoute, op, slog.String("listing_id", l.ID.String()))
}

func (h *ProposeTradeHandler) quoteSale(
	ctx context.Context, l multichain.Listing, micros, units uint64,
) (uint64, error) {
	const op = "governance.quoteSale"
	if micros == 0 {
		return 0, nil
	}
	q, err := h.ports.Quoter.Quote(ctx, l, multichain.SideSell, money.MicrosFromUint64(micros))
	if err != nil {
		return 0, err
	}
	scaled := new(big.Int).Mul(new(big.Int).SetUint64(micros), new(big.Int).SetUint64(units))
	if q.GrossOutput == nil || q.GrossOutput.Sign() <= 0 || !scaled.Quo(scaled, q.GrossOutput).IsUint64() {
		return 0, errs.New(errs.CodeDecodeFailed, op, slog.String("listing_id", l.ID.String()))
	}
	return scaled.Uint64(), nil
}

func saleComparison(l multichain.Listing, proceeds, units uint64, now time.Time) *multichain.Comparison {
	return &multichain.Comparison{
		StockID: l.StockID, Side: multichain.SideSell, Winner: l.ID,
		Ranked: []multichain.Quote{{
			ListingID: l.ID, Issuer: l.Issuer, Network: l.Network, Venue: l.Venue, Side: multichain.SideSell,
			NetShares: soldShares(l, units), NetMicros: money.MicrosFromUint64(proceeds), QuotedAt: now,
			ExpiresAt: now.Add(saleQuoteTTL),
		}},
	}
}

func soldShares(l multichain.Listing, units uint64) multichain.ShareNanos {
	if l.SharesPerToken == nil {
		return 0
	}
	whole := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(l.Decimals)), nil)
	shares := new(big.Rat).SetFrac(new(big.Int).SetUint64(units), whole)
	shares.Mul(shares, l.SharesPerToken)
	shares.Mul(shares, new(big.Rat).SetInt64(shareNanos))
	nanos := new(big.Int).Quo(shares.Num(), shares.Denom())
	if !nanos.IsUint64() {
		return 0
	}
	return multichain.ShareNanos(nanos.Uint64())
}
