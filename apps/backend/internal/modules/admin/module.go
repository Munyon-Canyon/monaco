package admin

import (
	"go.opentelemetry.io/otel"

	"github.com/monaco/monaco/apps/backend/internal/modules/admin/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	adminapi "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/adminapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type Module struct{ deps module.Deps }

func New(deps module.Deps) *Module { return &Module{deps: deps} }

func (*Module) Name() string { return "admin" }

func (m *Module) Mount(mount api.Mount) {
	adminapi.Mount(adapters.HTTP{
		Pool:    m.deps.Pool,
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
