package system

import (
	"github.com/monaco/monaco/apps/backend/internal/modules/system/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/system/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type Module struct {
	deps module.Deps
}

func New(d module.Deps) *Module { return &Module{deps: d} }

func (*Module) Name() string { return "system" }

func (m *Module) Routes(r *httpx.Routes) {
	r.SystemRoutes = adapters.HTTP{Record: app.NewRecordPingHandler(m.deps.UoW), Reads: m.deps.Pool, IDs: m.deps.IDs}
}

func (m *Module) Consumers() []bus.Consumer {
	echo := adapters.Echo{Hints: m.deps.Bus}
	return []bus.Consumer{
		{Durable: "system_echo", Handlers: []bus.HandlerSpec{bus.Handle("system.echo", echo.Handle)}},
	}
}

func (*Module) Pollers() []poller.Poller { return nil }
