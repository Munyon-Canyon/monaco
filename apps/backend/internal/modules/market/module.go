package market

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/jupiterprices"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/jupiterquote"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/mintfacts"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/xstocks"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type (
	Asset   = domain.Asset
	AssetID = domain.AssetID
	Mint    = domain.Mint
	Price   = domain.Sample
)

type Catalog interface {
	AssetByID(ctx context.Context, id AssetID) (Asset, error)
	AssetByMint(ctx context.Context, mint Mint) (Asset, error)
	AssetBySymbol(ctx context.Context, symbol string) (Asset, error)
	ListTradable(ctx context.Context) ([]Asset, error)
	ListAll(ctx context.Context) ([]Asset, error)
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
}

func New(d module.Deps) *Module { return &Module{deps: d} }

func (*Module) Name() string { return "market" }

var _ Catalog = (*app.Catalog)(nil)

func (m *Module) Catalog() *app.Catalog { return app.NewCatalog(m.deps.Pool) }

var _ Calendar = (*app.Calendar)(nil)

func (m *Module) Calendar() *app.Calendar { return app.NewCalendar(m.Catalog()) }

var _ Prices = (*app.PriceBook)(nil)

func (m *Module) Prices() *app.PriceBook { return app.NewPriceBook(m.deps.Pool, m.deps.Clock) }

var _ Routes = (*app.RouteChecker)(nil)

func (m *Module) RouteChecker() *app.RouteChecker {
	quoter := jupiterquote.New(jupiter.New(m.deps.Config, m.deps.Clock))
	return app.NewRouteChecker(m.Catalog(), quoter, m.deps.Clock)
}

func (*Module) Routes(*httpx.Routes) {}

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
	)
	facts := mintfacts.New(solana.New(cfg, m.deps.Clock))
	return []poller.Poller{
		app.NewCatalogPoller(m.deps.UoW, m.deps.Pool, m.deps.IDs, m.deps.Clock, providers, facts),
		m.samplePrices(),
		app.NewRetention(m.deps.UoW, m.deps.Clock),
	}
}

func (m *Module) samplePrices() *app.SamplePrices {
	cfg := m.deps.Config
	source := jupiterprices.New(jupiter.New(cfg, m.deps.Clock))
	return app.NewSamplePrices(m.deps.UoW, m.deps.Pool, m.deps.Clock, source, m.deps.Bus, cfg.Market.PricePollInterval)
}
