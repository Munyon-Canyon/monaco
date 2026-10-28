package ranking

import (
	"context"
	"time"

	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	fundingport "github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
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
	dbsqlc "github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/rankingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const (
	pageCacheSize  = 512
	curveCacheSize = 256
)

type Module struct {
	deps      module.Deps
	ports     app.Ports
	cabals    app.CabalStatus
	pages     adapters.PageCache
	curves    adapters.HistoryCache
	ledger    app.Contributions
	stakes    app.StakeHistory
	cards     app.CabalCards
	users     app.Users
	member    app.MemberCabals
	ownership app.Ownership
	roles     app.MemberRoles
}

type Option func(*Module)

func WithPorts(ports app.Ports) Option { return func(m *Module) { m.ports = ports } }

func New(d module.Deps, opts ...Option) *Module {
	m := &Module{
		deps:   d,
		ports:  app.Ports{Previous: sqlc.New(d.Pool), Snapshots: sqlc.New(d.Pool)},
		pages:  adapters.PageCache{Cache: adapters.NewCache[app.PageKey, domain.BoardPage](pageCacheSize)},
		curves: adapters.HistoryCache{Cache: adapters.NewCache[app.HistoryKey, domain.ValueHistory](curveCacheSize)},
	}
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
			m.ledger = provider.Queries()
			m.stakes = provider.Queries()
			m.ownership = provider.Queries()
		case interface{ Pauses() fundingport.Pauses }:
			m.ports.Funding = provider.Pauses()
		case interface{ Queries() cabalport.Queries }:
			m.ports.Cabals = provider.Queries()
			m.cabals = provider.Queries()
			m.cards = provider.Queries()
			m.member = provider.Queries()
			m.roles = provider.Queries()
		case interface{ Queries() identityport.Queries }:
			m.ports.Users = provider.Queries()
			m.users = provider.Queries()
		case interface{ Ballots() governanceport.Ballots }:
			m.ports.Ballots = provider.Ballots()
		case interface{ FollowGraph() app.Follows }:
			m.ports.Follows = provider.FollowGraph()
		case *market.Module:
			m.ports.Market = marketPort{
				catalog:  provider.Catalog(),
				prices:   provider.Prices(),
				listings: provider.Prices(),
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
	boards := adapters.Boards{DB: m.deps.Pool}
	rankingapi.Mount(adapters.HTTP{
		Boards: boards, Cabals: app.CheckCabal(m.cabals), Pages: m.pages, Follows: follows,
		Snapshots: boards, Ledger: m.ledger, Histories: m.curves, Stakes: m.stakes, Cards: m.cards,
		Users: m.users, Members: m.member, Ownership: m.ownership, Roles: m.roles,
	}, r)
}

func (m *Module) Consumers() []bus.Consumer { return m.consumers() }

func (m *Module) consumers() []bus.Consumer {
	return []bus.Consumer{membership(), m.names(), m.hints(), triggers(), m.calls()}
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

func (m *Module) Queries() port.Queries { return QueriesOn(m.deps.Pool) }

func (m *Module) Calls() interface {
	Records(
		ctx context.Context, userIDs []ids.UserID, cabalID *ids.CabalID, p port.Period,
	) (map[ids.UserID]port.CallRecord, error)
	CabalRecords(ctx context.Context, cabalIDs []ids.CabalID, p port.Period) (map[ids.CabalID]port.CallRecord, error)
} {
	return app.NewCallRecords(m.deps.Pool)
}

func (m *Module) CabalReturns() adapters.Latest { return adapters.Latest{DB: m.deps.Pool} }

func QueriesOn(db dbsqlc.DBTX) port.Queries { return adapters.Latest{DB: db} }

type marketPort struct {
	catalog  market.Catalog
	prices   market.Prices
	listings market.ListingPrices
}

func (p marketPort) AssetsByMints(ctx context.Context, mints []string) ([]market.Asset, error) {
	return p.catalog.ByMints(ctx, mints)
}

func (p marketPort) PricesAsOf(
	ctx context.Context,
	ids []market.AssetID,
	at time.Time,
) (map[market.AssetID]market.Price, error) {
	return p.prices.PricesAsOf(ctx, ids, at)
}

func (p marketPort) ListingPrices(
	ctx context.Context, keys []market.ListingKey, at time.Time,
) (map[market.ListingKey]market.ListingPrice, error) {
	return p.listings.ListingPrices(ctx, keys, at)
}
