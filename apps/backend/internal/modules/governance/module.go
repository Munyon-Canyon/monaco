package governance

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	dbsqlc "github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/governanceapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type (
	Status     = domain.Status
	ProposalID = ids.ProposalID
)

type Port = port.Queries

var _ Port = app.Queries{}

type Ports = app.TradePorts

type Option func(*Module)

func WithPorts(p Ports) Option { return func(m *Module) { m.ports = &p } }

type Module struct {
	deps  module.Deps
	ports *Ports
}

func New(d module.Deps, opts ...Option) *Module {
	m := &Module{deps: d}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func (*Module) Name() string { return "governance" }

func (m *Module) Mount(r api.Mount) {
	ports := m.tradePorts()
	hints := adapters.Hints{Publish: m.deps.Bus}
	governanceapi.Mount(adapters.HTTP{
		Propose:  app.NewProposeTradeHandler(m.deps.UoW, m.deps.IDs, m.deps.Clock, ports, hints),
		Vote:     app.NewCastVoteHandler(m.deps.UoW, m.deps.Clock, hints),
		Withdraw: app.NewWithdrawProposalHandler(m.deps.UoW, m.deps.Clock, hints),
		Reads:    app.NewProposalReads(m.deps.Pool, trading.New(m.deps).Queries()),
	}, r)
}

func (m *Module) tradePorts() Ports {
	if m.ports != nil {
		return *m.ports
	}
	deps := m.deps
	if deps.Config.Jupiter.SwapBaseURL == "" {
		deps.Config.Jupiter.SwapBaseURL = "http://jupiter.invalid"
	}
	if deps.Config.Jupiter.PriceBaseURL == "" {
		deps.Config.Jupiter.PriceBaseURL = "http://jupiter.invalid"
	}
	if deps.Config.Solana.RPCURL == "" {
		deps.Config.Solana.RPCURL = "http://solana.invalid"
	}
	if deps.Config.Timeouts.RPC == 0 {
		deps.Config.Timeouts.RPC = time.Second
	}
	if deps.Config.Timeouts.JupiterQuote == 0 {
		deps.Config.Timeouts.JupiterQuote = time.Second
	}
	if deps.Config.Timeouts.JupiterExecute == 0 {
		deps.Config.Timeouts.JupiterExecute = time.Second
	}
	markets := market.New(deps)
	return Ports{
		Cabals: cabal.New(m.deps).Queries(), Assets: markets.Catalog(), Routes: markets.RouteChecker(),
		Treasury: treasury.New(m.deps).Queries(), Balances: solana.New(deps.Config, deps.Clock),
	}
}

func (m *Module) Consumers() []bus.Consumer {
	hints := adapters.Hints{Publish: m.deps.Bus}
	outcomes := adapters.TradeOutcome{Hints: hints}
	return []bus.Consumer{
		{
			Durable: "governance",
			Handlers: []bus.HandlerSpec{
				bus.Handle("governance.trade_outcome.confirmed", outcomes.Confirmed),
				bus.Handle("governance.trade_outcome.blocked", outcomes.Blocked),
				bus.Handle("governance.trade_outcome.failed", outcomes.Failed),
				bus.Handle("governance.trade_outcome.retried", outcomes.Retried),
			},
		},
	}
}

func (m *Module) Pollers() []poller.Poller {
	hints := adapters.Hints{Publish: m.deps.Bus}
	return []poller.Poller{app.NewExpiryPoller(m.deps.UoW, m.deps.Pool, m.deps.Clock, hints)}
}

func (m *Module) Queries() port.Queries { return app.NewQueries(m.deps.Pool) }

func (*Module) DashboardOn(db dbsqlc.DBTX) app.Dashboard { return app.NewDashboard(db) }

func (m *Module) Voters() port.Voters { return app.NewQueries(m.deps.Pool) }

func (m *Module) ProposedMints(ctx context.Context) ([]chain.SolanaAddress, error) {
	return adapters.NewProposedMints(m.deps.Pool).ProposedMints(ctx)
}

func (m *Module) VoidFromOps(ctx context.Context, id ProposalID, rawReason string) error {
	reason, err := domain.ParseVoidReason(rawReason)
	if err != nil {
		return err
	}
	ctx = auth.WithActor(ctx, auth.Actor{Kind: auth.ActorSystem, ID: "monacoctl"})
	voids := app.NewVoidProposalHandler(
		m.deps.UoW, m.deps.Pool, m.deps.Clock, trading.New(m.deps).Queries(), adapters.Hints{Publish: m.deps.Bus},
	)
	return voids.Handle(ctx, app.VoidProposal{ProposalID: id, Reason: reason})
}
