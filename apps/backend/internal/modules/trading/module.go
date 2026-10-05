package trading

import (
	"context"
	"sync"

	"github.com/monaco/monaco/apps/backend/internal/events"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	fundingport "github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/adapters"
	chainadapters "github.com/monaco/monaco/apps/backend/internal/modules/trading/adapters/chain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/tradingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type (
	SwapView       = app.SwapView
	Source         = domain.Source
	SwapLayer      = app.SwapLayer
	SwapLayerDeps  = app.SwapLayerDeps
	SwapRequest    = app.SwapRequest
	TreasuryWallet = app.TreasuryWallet
	Venue          = app.Venue
	Signer         = app.Signer
	EnginePorts    = app.EnginePorts
)

const usdcDecimals = 6

type Queries interface {
	Swap(ctx context.Context, id ids.SwapID) (SwapView, error)
	SwapBySignature(ctx context.Context, sig chain.Signature) (SwapView, error)
	LatestBySource(ctx context.Context, src Source) (SwapView, bool, error)
	HasLiveSwap(ctx context.Context, src Source) (bool, error)
	OwnsSignature(ctx context.Context, sig chain.Signature) (bool, error)
}

type Module struct {
	deps   module.Deps
	ports  EnginePorts
	venue  Venue
	signer Signer
	once   sync.Once
	trades *app.ExecuteTradeHandler
	sells  *app.SellForCashOutHandler
}

type Option func(*Module)

func WithEnginePorts(p EnginePorts) Option { return func(m *Module) { m.ports = p } }

func WithChain(venue Venue, signer Signer) Option {
	return func(m *Module) { m.venue, m.signer = venue, signer }
}

func New(d module.Deps, opts ...Option) *Module {
	m := &Module{deps: d}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func (*Module) Name() string { return "trading" }

func (m *Module) Wire(set module.Set) {
	for _, mod := range set {
		switch provider := mod.(type) {
		case interface{ Queries() cabalport.Queries }:
			if m.ports.Cabals == nil {
				m.ports.Cabals = provider.Queries()
			}
		case interface{ Queries() governanceport.Queries }:
			if m.ports.Proposals == nil {
				m.ports.Proposals = provider.Queries()
			}
		case interface{ Queries() treasuryport.Queries }:
			if m.ports.Positions == nil {
				m.ports.Positions = provider.Queries()
			}
		case interface{ Pauses() fundingport.Pauses }:
			if m.ports.Pauses == nil {
				m.ports.Pauses = provider.Pauses()
			}
		}
	}
}

func (m *Module) Mount(r api.Mount) {
	ports := m.modulePorts()
	tradingapi.Mount(adapters.HTTP{
		Retry: app.NewRetryTradeHandler(m.deps.UoW, m.deps.Pool, ports.Cabals, ports.Proposals),
		Swaps: app.NewSwapDetailReads(m.deps.Pool, ports.Cabals, ports.Catalog),
	}, r)
}

func (m *Module) Consumers() []bus.Consumer {
	return []bus.Consumer{{
		Durable: "trading",
		Handlers: []bus.HandlerSpec{
			bus.HandleOwn("trading.engine", m.execute),
			bus.HandleOwn("trading.engine.retry", m.retry),
		},
	}, {
		Durable:  "trading_cashout_sell",
		Handlers: []bus.HandlerSpec{bus.HandleOwn("trading.cashout_sell", m.cashOutSell)},
	}}
}

func (m *Module) cashOutSell(ctx context.Context, d bus.Delivery, ev events.CashOutStarted) error {
	if m.deps.Config.Trade.Engine == config.TradeEngineStub {
		return adapters.StubEngine{UoW: m.deps.UoW}.HandleCashOut(ctx, d, ev)
	}
	m.once.Do(m.build)
	return adapters.CashOutSell{Sells: m.sells, UoW: m.deps.UoW}.Handle(ctx, d, ev)
}

func (m *Module) execute(ctx context.Context, d bus.Delivery, ev events.ProposalPassed) error {
	return m.engineAdapter().Handle(ctx, d, ev)
}

func (m *Module) retry(ctx context.Context, d bus.Delivery, ev events.TradeRetryRequested) error {
	return m.engineAdapter().HandleRetry(ctx, d, ev)
}

func (m *Module) engineAdapter() adapters.Engine {
	if m.deps.Config.Trade.Engine == config.TradeEngineStub {
		return adapters.Engine{Stub: &adapters.StubEngine{UoW: m.deps.UoW}}
	}
	m.once.Do(m.build)
	return adapters.Engine{Trades: m.trades}
}

func (m *Module) Pollers() []poller.Poller {
	return []poller.Poller{app.NewSwapSweeper(m.deps.UoW, m.deps.Pool, m.deps.Clock,
		chainadapters.NewReader(solana.New(m.deps.Config, m.deps.Clock)), m.deps.Bus)}
}

func (m *Module) Queries() app.Queries { return app.NewQueries(m.deps.Pool) }

func (m *Module) SignatureOwner() app.Queries { return m.Queries() }

func (m *Module) build() {
	cfg, clk := m.deps.Config, m.deps.Clock
	venue := m.venue
	if venue == nil {
		venue = chainadapters.NewVenue(jupiter.New(cfg, clk))
	}
	signer := m.signer
	if signer == nil {
		client, err := privy.New(cfg, clk)
		signer = unsignable{err: err}
		if err == nil {
			signer = chainadapters.NewSigner(client)
		}
	}
	layer := app.NewSwapLayer(app.SwapLayerDeps{
		UoW: m.deps.UoW, Reads: m.deps.Pool, Clock: clk, IDs: m.deps.IDs, Venue: venue, Signer: signer,
		Hints: m.deps.Bus,
	})
	usdc := chain.Mint{Address: chain.SolanaAddress(cfg.Solana.USDCMint), Decimals: usdcDecimals}
	ports := m.enginePorts()
	m.trades = app.NewExecuteTradeHandler(app.ExecuteTradeDeps{
		Layer: layer, UoW: m.deps.UoW, Reads: m.deps.Pool, Venue: venue, Ports: ports, USDC: usdc,
	})
	m.sells = app.NewSellForCashOutHandler(app.SellForCashOutDeps{
		Layer: layer, UoW: m.deps.UoW, Reads: m.deps.Pool, Clock: clk, Venue: venue, USDC: usdc,
		Holdings: app.LedgerHoldings{Ledger: ports.Positions, Catalog: ports.Catalog, USDC: usdc},
		Wallets:  app.CabalWallets{Cabals: ports.Cabals},
	})
}

func (m *Module) enginePorts() EnginePorts {
	p := m.modulePorts()
	if p.Balances == nil {
		p.Balances = solana.New(m.deps.Config, m.deps.Clock)
	}
	return p
}

func (m *Module) modulePorts() EnginePorts {
	p := m.ports
	if p.Catalog == nil {
		p.Catalog = market.New(m.deps).Catalog()
	}
	if p.Cabals == nil {
		p.Cabals = app.UnwiredCabals{}
	}
	if p.Pauses == nil {
		p.Pauses = app.UnwiredPauses{}
	}
	if p.Proposals == nil {
		p.Proposals = app.UnwiredProposals{}
	}
	if p.Positions == nil {
		p.Positions = app.UnwiredPositions{}
	}
	return p
}

type unsignable struct{ err error }

func (u unsignable) Sign(context.Context, string, []byte) ([]byte, chain.Signature, error) {
	return nil, "", u.err
}
