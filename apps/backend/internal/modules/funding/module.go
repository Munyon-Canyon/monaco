package funding

import (
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type Module struct{ deps module.Deps }

func New(d module.Deps) *Module { return &Module{deps: d} }

func (*Module) Name() string { return "funding" }

func (m *Module) Routes(r *httpx.Routes) {
	cfg := m.deps.Config
	r.FundingRoutes = adapters.HTTP{
		Create: app.NewCreateOnrampSessionHandler(m.deps.UoW, m.deps.Clock, cfg.FundPageURL()),
		Exchange: app.NewExchangeOnrampTokenHandler(m.deps.UoW, m.deps.Clock, identity.New(m.deps).Queries(),
			cfg.Solana.USDCMint),
		Report: app.NewReportOnrampStatusHandler(m.deps.UoW, m.deps.Clock, m.deps.Bus),
		Reads:  m.deps.Pool,
		IDs:    m.deps.IDs,
	}
}

func (*Module) Consumers() []bus.Consumer {
	return []bus.Consumer{}
}

func (m *Module) Pollers() []poller.Poller {
	cfg := m.deps.Config
	return []poller.Poller{
		app.NewDepositPoller(m.deps.Pool, m.deps.UoW, m.deps.IDs, m.deps.Clock,
			identity.New(m.deps).Queries(), solana.New(cfg, m.deps.Clock), chain.SolanaAddress(cfg.Solana.USDCMint),
			cfg.Funding.DepositPollInterval, cfg.Funding.DepositRPCRate, m.deps.Bus),
		app.NewOnrampExpiryPoller(m.deps.UoW, m.deps.Clock),
	}
}

func (*Module) Balances() port.Balances { return adapters.UnwiredBalances{} }

type (
	Balances = port.Balances
	Balance  = port.Balance
)
