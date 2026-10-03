package governance

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type (
	Status     = domain.Status
	ProposalID = ids.ProposalID
)

type Port interface {
	Status(ctx context.Context, id ProposalID) (Status, error)
}

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

func (m *Module) Routes(r *httpx.Routes) {
	ports := m.tradePorts()
	thresholds := cabalThresholds{cabals: ports.Cabals}
	r.GovernanceRoutes = adapters.HTTP{
		Propose:  app.NewProposeTradeHandler(m.deps.UoW, m.deps.IDs, m.deps.Clock, ports),
		Vote:     app.NewCastVoteHandler(m.deps.UoW, m.deps.Pool, m.deps.Clock, thresholds),
		Withdraw: app.NewWithdrawProposalHandler(m.deps.UoW, m.deps.Clock),
		Reads:    app.NewProposalReads(m.deps.Pool, thresholds, trading.New(m.deps).Queries()),
	}
}

func (m *Module) tradePorts() Ports {
	if m.ports != nil {
		return *m.ports
	}
	markets := market.New(m.deps)
	return Ports{
		Cabals: cabal.New(m.deps).Queries(), Assets: markets.Catalog(), Routes: markets.RouteChecker(),
		Treasury: treasury.New(m.deps).Queries(),
	}
}

func (*Module) Consumers() []bus.Consumer {
	return []bus.Consumer{}
}

func (m *Module) Pollers() []poller.Poller {
	return []poller.Poller{app.NewExpiryPoller(m.deps.UoW, m.deps.Pool, m.deps.Clock)}
}

func (m *Module) Queries() app.Queries { return app.NewQueries(m.deps.Pool) }

func (m *Module) VoidFromOps(ctx context.Context, id ProposalID, rawReason string) error {
	reason, err := domain.ParseVoidReason(rawReason)
	if err != nil {
		return err
	}
	ctx = auth.WithActor(ctx, auth.Actor{Kind: auth.ActorSystem, ID: "monacoctl"})
	voids := app.NewVoidProposalHandler(m.deps.UoW, m.deps.Pool, m.deps.Clock, trading.New(m.deps).Queries())
	return voids.Handle(ctx, app.VoidProposal{ProposalID: id, Reason: reason})
}
