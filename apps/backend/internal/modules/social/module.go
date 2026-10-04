package social

import (
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
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

func (m *Module) Mount(r api.Mount) { socialapi.Mount(m.http(), r) }

func (m *Module) http() adapters.HTTP {
	return adapters.HTTP{
		Follow: app.NewFollowHandler(app.FollowDeps{
			UoW: m.deps.UoW, Users: m.users, IDs: m.deps.IDs, Clock: m.deps.Clock,
		}),
		Unfollow: app.NewUnfollowHandler(m.deps.UoW, m.deps.Clock),
		Reads:    m.deps.Pool,
	}
}

func (m *Module) Consumers() []bus.Consumer {
	feed := adapters.Feed{Bus: m.deps.Bus, Users: m.users, IDs: m.deps.IDs}
	return []bus.Consumer{
		{
			Durable: "social_feed",
			Handlers: []bus.HandlerSpec{
				bus.HandleFetched("social.feed", feed.FetchCreated, feed.ApplyCreated),
			},
		},
	}
}

func (*Module) Pollers() []poller.Poller { return nil }
