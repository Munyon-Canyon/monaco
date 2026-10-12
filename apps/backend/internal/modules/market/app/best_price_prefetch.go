package app

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const (
	bestChainOp     = "market.BestChains"
	BestChainFresh  = 20 * time.Second
	BestChainServed = 60 * time.Second
	bestChainBudget = 10 * time.Second
	bestChainProbe  = 1_000_000
)

func BestChainProbe() money.Micros { return money.MicrosFromUint64(bestChainProbe) }

type BestChainAssets interface {
	AssetBySymbol(ctx context.Context, symbol string) (domain.Asset, error)
}

type BestChainer interface {
	Get(ctx context.Context, symbol string) (BestChain, error)
}

type BestChain struct {
	Symbol     string
	Amount     money.Micros
	QuotedAt   time.Time
	Comparison multichain.Comparison
}

func (b BestChain) Winner() multichain.Quote { return b.Comparison.Ranked[0] }

type BestChains struct {
	assets BestChainAssets
	router multichain.Router
	clock  clock.Clock
	mu     sync.Mutex
	held   map[string]BestChain
	flight singleflight.Group
}

var _ BestChainer = (*BestChains)(nil)

func NewBestChains(assets BestChainAssets, router multichain.Router, clk clock.Clock) *BestChains {
	return &BestChains{assets: assets, router: router, clock: clk, held: map[string]BestChain{}}
}

func (b *BestChains) Get(ctx context.Context, symbol string) (BestChain, error) {
	if got, ok := b.cached(ctx, symbol); ok {
		return got, nil
	}
	select {
	case <-ctx.Done():
		return BestChain{}, errs.Wrap(ctx.Err(), errs.CodeInternal, bestChainOp)
	case res := <-b.flight.DoChan(symbol, b.refresh(ctx, symbol)):
		if res.Err != nil {
			return BestChain{}, res.Err
		}
		return res.Val.(BestChain), nil
	}
}

func (b *BestChains) cached(ctx context.Context, symbol string) (BestChain, bool) {
	now := b.clock.Now()
	b.mu.Lock()
	got, ok := b.held[symbol]
	b.mu.Unlock()
	age := now.Sub(got.QuotedAt)
	switch {
	case !ok || age >= BestChainServed:
		return BestChain{}, false
	case age >= BestChainFresh:
		b.flight.DoChan(symbol, b.refresh(ctx, symbol))
	}
	return got, true
}

func (b *BestChains) refresh(ctx context.Context, symbol string) func() (any, error) {
	return func() (any, error) {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), bestChainBudget)
		defer cancel()
		start := b.clock.Now()
		c, err := b.compare(ctx, symbol)
		if err != nil {
			return nil, err
		}
		got := BestChain{Symbol: symbol, Amount: BestChainProbe(), QuotedAt: start, Comparison: c}
		b.mu.Lock()
		b.held[symbol] = got
		b.mu.Unlock()
		winner := got.Winner()
		observability.Info(ctx, observability.MarketBestChainRefreshed,
			slog.String("symbol", symbol), slog.String("network", string(winner.Network)),
			slog.String("venue", string(winner.Venue)),
			slog.Int64("duration_ms", b.clock.Now().Sub(start).Milliseconds()))
		return got, nil
	}
}

func (b *BestChains) compare(ctx context.Context, symbol string) (multichain.Comparison, error) {
	asset, err := b.assets.AssetBySymbol(ctx, symbol)
	if err != nil {
		return multichain.Comparison{}, err
	}
	if !asset.Tradable() {
		return multichain.Comparison{}, errs.New(errs.CodeAssetUntradable, bestChainOp, slog.String("symbol", symbol))
	}
	var stock multichain.StockID
	if err := stock.UnmarshalText([]byte(asset.ID.String())); err != nil {
		return multichain.Comparison{}, errs.Wrap(err, errs.CodeDecodeFailed, bestChainOp)
	}
	return b.router.Best(ctx, stock, multichain.SideBuy, BestChainProbe())
}
