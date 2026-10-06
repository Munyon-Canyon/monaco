package notify

import (
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/notifyapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type Module struct {
	deps   module.Deps
	users  app.Users
	sender apns.Sender
}

type Option func(*Module)

func WithUsers(users app.Users) Option {
	return func(m *Module) { m.users = users }
}

func WithSender(sender apns.Sender) Option {
	return func(m *Module) { m.sender = sender }
}

func New(d module.Deps, opts ...Option) *Module {
	m := &Module{deps: d, users: identity.New(d).Queries(), sender: d.APNs}
	for _, opt := range opts {
		opt(m)
	}
	if m.sender == nil {
		m.sender = apns.NoopSender{}
	}
	return m
}

func (*Module) Name() string { return "notify" }

func (m *Module) Mount(r api.Mount) {
	notifyapi.Mount(adapters.HTTP{
		Register:   app.NewRegisterDeviceHandler(m.deps.UoW, m.users, m.deps.IDs, m.deps.Clock),
		Unregister: app.NewUnregisterDeviceHandler(m.deps.UoW),
	}, r)
}

func (m *Module) Consumers() []bus.Consumer {
	pusher := app.NewPusher(m.deps.UoW, m.users, m.sender, m.deps.IDs, m.deps.Clock)
	return []bus.Consumer{{Durable: "notify", Handlers: []bus.HandlerSpec{adapters.Push(pusher, app.Test{})}}}
}

func (*Module) Pollers() []poller.Poller { return nil }
