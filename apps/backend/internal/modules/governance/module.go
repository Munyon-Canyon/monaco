package governance

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
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

type Port = port.Queries

var _ Port = app.Queries{}

type Module struct {
	deps module.Deps
}

func New(d module.Deps) *Module { return &Module{deps: d} }

func (*Module) Name() string { return "governance" }

func (m *Module) Routes(r *httpx.Routes) {
	thresholds := cabalThresholds{cabals: cabal.New(m.deps).Queries()}
	r.GovernanceRoutes = adapters.HTTP{
		Vote:     app.NewCastVoteHandler(m.deps.UoW, m.deps.Pool, m.deps.Clock, thresholds),
		Withdraw: app.NewWithdrawProposalHandler(m.deps.UoW, m.deps.Clock),
		Reads:    app.NewProposalReads(m.deps.Pool, thresholds, trading.New(m.deps).Queries()),
	}
}

func (*Module) Consumers() []bus.Consumer {
	return []bus.Consumer{}
}

func (m *Module) Pollers() []poller.Poller {
	return []poller.Poller{app.NewExpiryPoller(m.deps.UoW, m.deps.Pool, m.deps.Clock)}
}

func (m *Module) Queries() port.Queries { return app.NewQueries(m.deps.Pool) }

func (m *Module) VoidFromOps(ctx context.Context, id ProposalID, rawReason string) error {
	reason, err := domain.ParseVoidReason(rawReason)
	if err != nil {
		return err
	}
	ctx = auth.WithActor(ctx, auth.Actor{Kind: auth.ActorSystem, ID: "monacoctl"})
	voids := app.NewVoidProposalHandler(m.deps.UoW, m.deps.Pool, m.deps.Clock, trading.New(m.deps).Queries())
	return voids.Handle(ctx, app.VoidProposal{ProposalID: id, Reason: reason})
}
