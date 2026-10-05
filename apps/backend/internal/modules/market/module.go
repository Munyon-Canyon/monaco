package market

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/metric"

	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/coingecko"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/jupiterprices"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/jupiterquote"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/mintfacts"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/prestocks"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/tessera"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/xstocks"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/marketapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type (
	Asset      = domain.Asset
	AssetID    = domain.AssetID
	Mint       = domain.Mint
	Price      = domain.Sample
	Multiplier = domain.Multiplier
)

func ParseMint(raw string) (Mint, error) { return domain.ParseMint(raw) }

type Catalog interface {
	AssetByID(ctx context.Context, id AssetID) (Asset, error)
	AssetByMint(ctx context.Context, mint Mint) (Asset, error)
	AssetBySymbol(ctx context.Context, symbol string) (Asset, error)
	ListTradable(ctx context.Context) ([]Asset, error)
	ListAll(ctx context.Context) ([]Asset, error)
}

type CompanyListings interface {
	Siblings(ctx context.Context, id AssetID) ([]Asset, error)
}

type SessionInfo = domain.SessionInfo

type Calendar interface {
	Session(ctx context.Context, id AssetID, at time.Time) (SessionInfo, error)
}

type Prices interface {
	LatestPrices(ctx context.Context) (map[AssetID]Price, error)
	PricesAsOf(ctx context.Context, ids []AssetID, at time.Time) (map[AssetID]Price, error)
}

type Side = app.Side

const (
	SideBuy  = app.SideBuy
	SideSell = app.SideSell
)

type RouteCheck = app.RouteCheck

type Routes interface {
	CheckRoute(ctx context.Context, id AssetID, side Side, amount money.BaseUnits) (RouteCheck, error)
}

type Module struct {
	deps module.Deps
	hot  []app.HotMints
}

func New(d module.Deps) *Module { return &Module{deps: d} }

func (*Module) Name() string { return "market" }

func (m *Module) Wire(set module.Set) {
	for _, mod := range set {
		switch provider := mod.(type) {
		case treasuryport.HeldMints:
			m.hot = append(m.hot, provider.HeldMints)
		case governanceport.ProposedMints:
			m.hot = append(m.hot, provider.ProposedMints)
		}
	}
}

var (
	_ Catalog         = (*app.Catalog)(nil)
	_ CompanyListings = (*app.Catalog)(nil)
)

func (m *Module) Catalog() *app.Catalog { return app.NewCatalog(m.deps.Pool) }

var _ Calendar = (*app.Calendar)(nil)

func (m *Module) Calendar() *app.Calendar { return app.NewCalendar(m.Catalog()) }

var _ Prices = (*app.PriceBook)(nil)

func (m *Module) Prices() *app.PriceBook { return app.NewPriceBook(m.deps.Pool, m.deps.Clock) }

var _ Routes = (*app.RouteChecker)(nil)

func (m *Module) RouteChecker() *app.RouteChecker {
	quoter := jupiterquote.New(m.deps.JupiterClient())
	return app.NewRouteChecker(m.Catalog(), quoter, m.deps.Clock)
}

func (m *Module) Mount(r api.Mount) {
	list := app.NewListAssets(m.deps.Pool, m.deps.Clock)
	cache := adapters.NewCache(m.deps.Clock)
	marketapi.Mount(adapters.HTTP{
		List:   adapters.CacheList(list, cache),
		Detail: app.NewDetail(m.deps.Pool, list),
		Chart:  adapters.CacheChart(app.NewChart(m.deps.Pool, m.deps.Clock), cache),
		Clock:  m.deps.Clock,
	}, r)
}

func (*Module) Consumers() []bus.Consumer { return nil }

func (m *Module) Pollers() []poller.Poller {
	cfg := m.deps.Config
	providers := app.NewProviders(
		xstocks.New(m.deps.HTTPClient(
			"xstocks",
			httpclient.WithBaseURL(cfg.XStocks.BaseURL),
			httpclient.WithTimeout(
				cfg.Timeouts.XStocks,
			),
			httpclient.WithRetry(3, 250*time.Millisecond, 2*time.Second),
		)),
		tessera.New(m.deps.HTTPClient(
			"tessera",
			httpclient.WithBaseURL(cfg.Tessera.BaseURL),
			httpclient.WithTimeout(cfg.Timeouts.Tessera),
			httpclient.WithRetry(3, 250*time.Millisecond, 2*time.Second),
		)),
		prestocks.New(m.deps.HTTPClient(
			"prestocks",
			httpclient.WithBaseURL(cfg.PreStocks.BaseURL),
			httpclient.WithTimeout(cfg.Timeouts.PreStocks),
			httpclient.WithRetry(3, 250*time.Millisecond, 2*time.Second),
		)),
	)
	facts := mintfacts.New(solana.New(cfg, m.deps.Clock))
	history := m.priceHistory()
	return []poller.Poller{
		app.NewCatalogPoller(m.deps.UoW, m.deps.Pool, m.deps.IDs, m.deps.Clock, providers, facts),
		m.samplePrices(),
		app.NewRetention(m.deps.UoW, m.deps.Clock),
		app.NewBackfill(m.deps.UoW, m.deps.Pool, m.deps.Clock, history),
		app.NewReconcile(m.deps.UoW, m.deps.Pool, history),
	}
}

func (m *Module) priceHistory() *coingecko.Client {
	cfg := m.deps.Config
	return coingecko.New(m.deps.HTTPClient("coingecko", coingecko.Options(cfg)...), cfg.CoinGecko.APIKey)
}

func (m *Module) PriceHints(ctx context.Context, meter metric.Meter) (func(), error) {
	hints, err := app.NewPriceHints(m.deps.Bus, m.deps.Clock, meter)
	if err != nil {
		return nil, err
	}
	return hints.Subscribe(ctx)
}

func (m *Module) samplePrices() *app.SamplePrices {
	cfg := m.deps.Config
	source := jupiterprices.New(m.deps.JupiterClient())
	return app.NewSamplePrices(
		m.deps.UoW, m.deps.Pool, m.deps.IDs, m.deps.Clock, source, m.deps.Bus, cfg.Market.PricePollInterval,
		m.hot...,
	)
}
