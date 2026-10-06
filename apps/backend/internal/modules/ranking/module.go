package ranking

import (
	"context"
	"time"

	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	fundingport "github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/rankingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const pageCacheSize = 512

type Module struct {
	deps   module.Deps
	ports  app.Ports
	cabals app.CabalStatus
	pages  adapters.PageCache
}

type Option func(*Module)

func WithPorts(ports app.Ports) Option { return func(m *Module) { m.ports = ports } }

func New(d module.Deps, opts ...Option) *Module {
	cache := adapters.NewCache[app.PageKey, domain.BoardPage](pageCacheSize)
	m := &Module{deps: d, ports: app.Ports{Previous: sqlc.New(d.Pool)}, pages: adapters.PageCache{Cache: cache}}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func (m *Module) Wire(set module.Set) {
	for _, mod := range set {
		switch provider := mod.(type) {
		case interface{ Queries() treasuryport.Queries }:
			m.ports.Treasury = provider.Queries()
		case interface{ Pauses() fundingport.Pauses }:
			m.ports.Funding = provider.Pauses()
		case interface{ Queries() cabalport.Queries }:
			m.ports.Cabals = provider.Queries()
			m.cabals = provider.Queries()
		case interface{ Queries() identityport.Queries }:
			m.ports.Users = provider.Queries()
		case interface{ FollowGraph() app.Follows }:
			m.ports.Follows = provider.FollowGraph()
		case *market.Module:
			m.ports.Market = marketPort{
				catalog:  provider.Catalog(),
				prices:   provider.Prices(),
				calendar: provider.Calendar(),
			}
		}
	}
}

func (m *Module) Ports() app.Ports { return m.ports }

func (m *Module) RunValuation() app.RunValuation {
	return app.NewRunValuation(m.ports, chain.SolanaAddress(m.deps.Config.Solana.USDCMint))
}

func (*Module) Name() string { return "ranking" }

func (m *Module) Mount(r api.Mount) {
	follows := m.ports.Follows
	if follows == nil {
		follows = app.UnwiredFollows{}
	}
	rankingapi.Mount(adapters.HTTP{
		Boards: adapters.Boards{DB: m.deps.Pool}, Cabals: app.CheckCabal(m.cabals), Pages: m.pages,
		Follows: follows,
	}, r)
}

func (m *Module) Consumers() []bus.Consumer { return m.consumers() }

func (m *Module) consumers() []bus.Consumer {
	return []bus.Consumer{membership(), m.names(), triggers()}
}

func (m *Module) Pollers() []poller.Poller {
	return []poller.Poller{app.NewThinSnapshots(m.deps.Pool, m.deps.Clock), m.valuation()}
}

func (m *Module) valuation() adapters.ValuationPoller {
	return adapters.ValuationPoller{
		Reads:  sqlc.New(m.deps.Pool),
		Runner: m.RunValuation(),
		Writer: app.NewSnapshotWriter(m.deps.UoW, m.deps.IDs),
		Clock:  m.deps.Clock,
	}
}

type Port = port.Queries

func (m *Module) Queries() port.Queries { return adapters.Latest{DB: m.deps.Pool} }

type marketPort struct {
	catalog  market.Catalog
	prices   market.Prices
	calendar market.Calendar
}

func (p marketPort) ListAll(ctx context.Context) ([]market.Asset, error) {
	return p.catalog.ListAll(ctx)
}

func (p marketPort) LatestPrices(ctx context.Context) (map[market.AssetID]market.Price, error) {
	return p.prices.LatestPrices(ctx)
}

func (p marketPort) PricesAsOf(
	ctx context.Context,
	ids []market.AssetID,
	at time.Time,
) (map[market.AssetID]market.Price, error) {
	return p.prices.PricesAsOf(ctx, ids, at)
}

func (p marketPort) Session(ctx context.Context, id market.AssetID, at time.Time) (market.SessionInfo, error) {
	return p.calendar.Session(ctx, id, at)
}
