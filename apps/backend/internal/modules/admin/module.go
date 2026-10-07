package admin

import (
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	adminapi "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/adminapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type Module struct{ deps module.Deps }

func New(deps module.Deps) *Module { return &Module{deps: deps} }

func (*Module) Name() string { return "admin" }

func (m *Module) Mount(mount api.Mount) { adminapi.Mount(adapters.HTTP{Pool: m.deps.Pool}, mount) }

func (*Module) Consumers() []bus.Consumer {
	return []bus.Consumer{
		{Durable: "admin", Handlers: []bus.HandlerSpec{bus.Handle("admin.audit", adapters.Audit{}.Handle)}},
	}
}

func (*Module) Pollers() []poller.Poller { return nil }
