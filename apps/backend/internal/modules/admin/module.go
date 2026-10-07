package admin

import (
	"go.opentelemetry.io/otel"

	"github.com/monaco/monaco/apps/backend/internal/modules/admin/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/app"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	adminapi "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/adminapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type Module struct {
	deps   module.Deps
	users  identityport.Queries
	cabals cabalport.Queries
}

func New(deps module.Deps) *Module { return &Module{deps: deps} }

func (m *Module) Wire(set module.Set) {
	for _, mod := range set {
		switch provider := mod.(type) {
		case interface{ Queries() identityport.Queries }:
			m.users = provider.Queries()
		case interface{ Queries() cabalport.Queries }:
			m.cabals = provider.Queries()
		}
	}
}

func (m *Module) userLookup() app.UserLookup {
	ledger := treasury.New(m.deps).Ledger()
	return app.UserLookup{
		Users: m.users, Wallets: m.users, Cabals: m.cabals, Shares: ledger, Txns: ledger,
		Events: bus.NewEventLog(m.deps.Pool, m.deps.Clock), Actions: adapters.ActionLog{DB: m.deps.Pool},
	}
}

func (m *Module) cabalLookup() app.CabalLookup {
	ledger := treasury.New(m.deps).Ledger()
	return app.CabalLookup{
		Cabals: m.cabals, Details: m.cabals, Users: m.users, Shares: ledger, Holdings: ledger,
		Assets: market.New(m.deps).Catalog(), Txns: ledger, Actions: adapters.ActionLog{DB: m.deps.Pool},
	}
}

func (*Module) Name() string { return "admin" }

func (m *Module) txnLookup() app.TxnLookup {
	swaps := trading.New(m.deps).Queries()
	return app.TxnLookup{
		Ledger: treasury.New(m.deps).Ledger(), Swaps: swaps, Requests: swaps,
		Events: bus.NewEventLog(m.deps.Pool, m.deps.Clock),
	}
}

func (m *Module) queues() app.Queues {
	return app.Queues{
		Swaps: trading.New(m.deps).Queries(), Events: bus.NewEventLog(m.deps.Pool, m.deps.Clock),
		Letters: adapters.DeadLetterCount{DB: m.deps.Pool}, Clock: m.deps.Clock,
	}
}

func (m *Module) Mount(mount api.Mount) {
	adminapi.Mount(adapters.HTTP{
		Pool:    m.deps.Pool,
		Users:   m.userLookup(),
		Cabals:  m.cabalLookup(),
		Txns:    m.txnLookup(),
		Queues:  m.queues(),
		Redrive: app.NewRedriveDeadLetterHandler(m.deps.UoW, m.deps.Pool, m.deps.Bus, m.deps.Clock),
		Discard: app.NewDiscardDeadLetterHandler(m.deps.UoW, m.deps.Clock),
	}, mount)
}

func (*Module) Consumers() []bus.Consumer {
	return []bus.Consumer{
		{Durable: "admin", Handlers: []bus.HandlerSpec{bus.Handle("admin.audit", adapters.Audit{}.Handle)}},
	}
}

func (m *Module) Pollers() []poller.Poller {
	meter := otel.GetMeterProvider().Meter("github.com/monaco/monaco/apps/backend/internal/modules/admin")
	return []poller.Poller{
		app.NewDeadLettersPoller(app.DeadLettersDeps{
			Source:   m.deps.Bus,
			Events:   adapters.BusEvents{Conn: m.deps.Bus},
			UoW:      m.deps.UoW,
			Reads:    m.deps.Pool,
			IDs:      m.deps.IDs,
			Clock:    m.deps.Clock,
			Meter:    meter,
			Interval: m.deps.Config.Admin.DeadLettersInterval,
		}),
	}
}
