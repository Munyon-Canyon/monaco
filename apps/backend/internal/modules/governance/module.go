package governance

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
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

type Module struct {
	deps module.Deps
}

func New(d module.Deps) *Module { return &Module{deps: d} }

func (*Module) Name() string { return "governance" }

func (m *Module) Routes(r *httpx.Routes) {
	thresholds := cabalThresholds{cabals: cabal.New(m.deps).Queries()}
	r.GovernanceRoutes = adapters.HTTP{
		Vote:  app.NewCastVoteHandler(m.deps.UoW, m.deps.Pool, m.deps.Clock, thresholds),
		Reads: app.NewProposalReads(m.deps.Pool, thresholds, trading.New(m.deps).Queries()),
	}
}

func (*Module) Consumers() []bus.Consumer {
	return []bus.Consumer{}
}

func (m *Module) Pollers() []poller.Poller {
	return []poller.Poller{app.NewExpiryPoller(m.deps.UoW, m.deps.Pool, m.deps.Clock)}
}

func (m *Module) Queries() app.Queries { return app.NewQueries(m.deps.Pool) }
