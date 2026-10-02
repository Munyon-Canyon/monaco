package notify

import (
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type Module struct {
	deps  module.Deps
	users app.Users
}

type Option func(*Module)

func WithUsers(users app.Users) Option {
	return func(m *Module) { m.users = users }
}

func New(d module.Deps, opts ...Option) *Module {
	m := &Module{deps: d, users: identity.New(d).Queries()}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func (*Module) Name() string { return "notify" }

func (m *Module) Routes(r *httpx.Routes) {
	r.NotifyRoutes = adapters.HTTP{
		Register:   app.NewRegisterDeviceHandler(m.deps.UoW, m.users, m.deps.IDs, m.deps.Clock),
		Unregister: app.NewUnregisterDeviceHandler(m.deps.UoW),
	}
}

func (*Module) Consumers() []bus.Consumer {
	return []bus.Consumer{}
}

func (*Module) Pollers() []poller.Poller { return nil }
