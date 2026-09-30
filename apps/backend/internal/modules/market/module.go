package market

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type (
	Asset   = domain.Asset
	AssetID = domain.AssetID
	Mint    = domain.Mint
)

type Catalog interface {
	AssetByID(ctx context.Context, id AssetID) (Asset, error)
	AssetByMint(ctx context.Context, mint Mint) (Asset, error)
	AssetBySymbol(ctx context.Context, symbol string) (Asset, error)
	ListTradable(ctx context.Context) ([]Asset, error)
	ListAll(ctx context.Context) ([]Asset, error)
}

type Module struct {
	deps module.Deps
}

func New(d module.Deps) *Module { return &Module{deps: d} }

func (*Module) Name() string { return "market" }

var _ Catalog = (*app.Catalog)(nil)

func (m *Module) Catalog() *app.Catalog { return app.NewCatalog(m.deps.Pool) }

func (*Module) Routes(*httpx.Routes) {}

func (*Module) Consumers() []bus.Consumer { return nil }

func (*Module) Pollers() []poller.Poller { return nil }
