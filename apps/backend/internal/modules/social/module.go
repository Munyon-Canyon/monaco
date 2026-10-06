package social

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters/ably"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type Module struct {
	deps     module.Deps
	users    app.Users
	members  app.Members
	realtime app.Realtime
}

type Option func(*Module)

func WithUsers(users app.Users) Option {
	return func(m *Module) { m.users = users }
}

func WithRealtime(realtime app.Realtime) Option {
	return func(m *Module) { m.realtime = realtime }
}

func New(d module.Deps, opts ...Option) *Module {
	m := &Module{deps: d}
	for _, opt := range opts {
		opt(m)
	}
	if m.users == nil {
		m.users = identity.New(d).Queries()
	}
	m.useDefaultRealtime()
	m.members = cabal.New(d).Queries()
	return m
}

func (m *Module) useDefaultRealtime() {
	if m.realtime != nil {
		return
	}
	if m.deps.Config.Ably.APIKey == "" {
		m.realtime = ably.Noop{}
		return
	}
	client, err := ably.New(m.deps.Config, m.deps.HTTPClient)
	if err != nil {
		panic(err)
	}
	m.realtime = client
}

type FollowsPort = app.FollowsPort

func (m *Module) Follows() app.Follows { return app.NewFollows(m.deps.Pool) }

func (m *Module) FollowCounts() interface {
	Counts(context.Context, ids.UserID) (int, int, error)
	FollowedByMe(context.Context, ids.UserID, ids.UserID) (bool, error)
} {
	return m.Follows()
}

func (*Module) Name() string { return "social" }

func (m *Module) Mount(r api.Mount) { socialapi.Mount(m.http(), r) }

func (m *Module) http() adapters.HTTP {
	chat := app.ChatDeps{
		UoW: m.deps.UoW, Members: m.members, IDs: m.deps.IDs, Clock: m.deps.Clock,
		Publish: app.NewChatPublisher(m.realtime, adapters.ChatWire(m.users)),
	}
	return adapters.HTTP{
		Follow: app.NewFollowHandler(app.FollowDeps{
			UoW: m.deps.UoW, Users: m.users, IDs: m.deps.IDs, Clock: m.deps.Clock,
		}),
		Unfollow:   app.NewUnfollowHandler(m.deps.UoW, m.deps.Clock),
		Mute:       app.NewMuteHandler(m.deps.UoW, m.deps.Clock),
		Unmute:     app.NewUnmuteHandler(m.deps.UoW),
		PostChat:   app.NewPostChatMessageHandler(chat),
		DeleteChat: app.NewDeleteChatMessageHandler(chat),
		Token:      app.NewRealtimeTokenHandler(m.members, m.realtime),
		CreateComment: app.NewCreateCommentHandler(app.CommentDeps{
			UoW: m.deps.UoW, Reads: m.deps.Pool, Members: m.members, IDs: m.deps.IDs, Clock: m.deps.Clock,
		}),
		Members: m.members,
		Reads:   m.deps.Pool,
		Users:   m.users,
	}
}

func (m *Module) Consumers() []bus.Consumer {
	feed := adapters.Feed{Bus: m.deps.Bus, Users: m.users, IDs: m.deps.IDs, UoW: m.deps.UoW}
	return []bus.Consumer{
		{
			Durable: "social_feed",
			Handlers: []bus.HandlerSpec{
				bus.HandleFetched("social.feed", feed.FetchCreated, feed.ApplyCreated),
				bus.HandleFetched("social.feed.joined", feed.FetchJoined, feed.ApplyJoined),
				bus.Handle("social.feed.left", feed.Left),
				bus.HandleFetched("social.feed.profile_updated", feed.FetchProfile, feed.ApplyProfile),
				bus.Handle("social.feed.cabal_updated", feed.CabalUpdated),
			},
		},
	}
}

func (*Module) Pollers() []poller.Poller { return nil }
