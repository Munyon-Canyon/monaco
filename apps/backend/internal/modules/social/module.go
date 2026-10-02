package social

import (
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
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
	m := &Module{deps: d}
	for _, opt := range opts {
		opt(m)
	}
	if m.users == nil {
		m.users = identity.New(d).Queries()
	}
	return m
}

func (*Module) Name() string { return "social" }

func (m *Module) Routes(r *httpx.Routes) {
	r.SocialRoutes = adapters.HTTP{
		Follow: app.NewFollowHandler(app.FollowDeps{
			UoW: m.deps.UoW, Users: m.users, IDs: m.deps.IDs, Clock: m.deps.Clock,
		}),
		Unfollow: app.NewUnfollowHandler(m.deps.UoW, m.deps.Clock),
	}
}

func (*Module) Consumers() []bus.Consumer {
	return []bus.Consumer{}
}

func (*Module) Pollers() []poller.Poller { return nil }
